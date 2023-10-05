package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"

	log "github.com/sirupsen/logrus"
)

// Config is the struct representation of gosh's YAML configuration file.
//
// For each field's meaning, please consider the gosh.yml file in this
// repository as it serves both as an example as well as documentation and
// otherwise the documentation will diverge anyways.
type Config struct {
	User  string
	Group string

	Store struct {
		Path string
	}

	Webserver struct {
		Listen struct {
			Protocol string
			Bound    string
		}

		UnixSocket struct {
			Chmod string
			Owner string
			Group string
		} `yaml:"unix_socket"`

		Protocol string

		UrlPrefix string `yaml:"url_prefix"`

		ItemConfig struct {
			MaxSize     string        `yaml:"max_size"`
			MaxLifetime time.Duration `yaml:"max_lifetime"`

			MimeDrop []string          `yaml:"mime_drop"`
			MimeMap  map[string]string `yaml:"mime_map"`
		} `yaml:"item_config"`

		Contact string
	}
}

// loadConfig loads a Config from a given YAML configuration file at the path.
func loadConfig(path string) (Config, error) {
	var conf Config

	f, err := os.Open(path)
	if err != nil {
		return conf, err
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&conf)
	return conf, err
}

// forkChild forks off a subprocess for the given child subroutine.
//
// The child process' output will be printed to this process' output. The
// extraFiles are additional file descriptors for communication.
func forkChild(child string, extraFiles []*os.File, ctx context.Context) (*exec.Cmd, error) {
	logParent, logChild, err := Socketpair()
	if err != nil {
		return nil, err
	}

	go func() {
		scanner := bufio.NewScanner(logParent)
		for scanner.Scan() {
			log.WithField("subprocess", child).Print(scanner.Text())
			if err := scanner.Err(); err != nil {
				log.WithField("subprocess", child).WithError(err).Error("Scanner failed")
			}
		}
	}()

	cmd := exec.CommandContext(ctx, os.Args[0], append(os.Args[1:], "-fork-child", child)...)

	cmd.Env = []string{}
	cmd.Stdin = nil
	cmd.Stdout = logChild
	cmd.Stderr = logChild
	cmd.ExtraFiles = extraFiles

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

// uidGidForUserGroup fetches an UID and GID for the given user and group.
func uidGidForUserGroup(username, groupname string) (uid, gid int, err error) {
	userStruct, err := user.Lookup(username)
	if err != nil {
		return
	}
	userId, err := strconv.ParseInt(userStruct.Uid, 10, 64)
	if err != nil {
		return
	}
	groupStruct, err := user.LookupGroup(groupname)
	if err != nil {
		return
	}
	groupId, err := strconv.ParseInt(groupStruct.Gid, 10, 64)
	if err != nil {
		return
	}

	uid, gid = int(userId), int(groupId)
	return
}

// posixPermDrop uses (more or less) POSIX defined options to drop privileges.
//
// Frist, a chroot is set to the given path. Afterwards, the effective UID and
// GID are being set to those of the given user and group.
//
// It says "more or less POSIX" as setresuid(2) and setresgid(2) aren't part of
// any standard (yet), but are supported by most operating systems.
func posixPermDrop(chroot, username, groupname string) error {
	uid, gid, err := uidGidForUserGroup(username, groupname)
	if err != nil {
		return err
	}

	err = unix.Chroot(chroot)
	if err != nil {
		return fmt.Errorf("chroot: %w", err)
	}
	err = unix.Chdir("/")
	if err != nil {
		return fmt.Errorf("chdir: %w", err)
	}

	err = unix.Setgroups([]int{gid})
	if err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	err = unix.Setresgid(gid, gid, gid)
	if err != nil {
		return fmt.Errorf("setresgid: %w", err)
	}
	err = unix.Setresuid(uid, uid, uid)
	if err != nil {
		return fmt.Errorf("setresuid: %w", err)
	}

	return nil
}

func mainMonitor(conf Config) {
	storeRpcServer, storeRpcClient, err := Socketpair()
	if err != nil {
		log.Fatal(err)
	}
	storeFdServer, storeFdClient, err := Socketpair()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), unix.SIGINT)
	defer cancel()

	_, err = forkChild("store", []*os.File{storeRpcServer, storeFdServer}, ctx)
	if err != nil {
		log.Fatal(err)
	}

	_, err = forkChild("webserver", []*os.File{storeRpcClient, storeFdClient}, ctx)
	if err != nil {
		log.Fatal(err)
	}

	bottomlessPit, err := os.MkdirTemp("", "gosh-monitor-chroot")
	if err != nil {
		log.WithError(err).Fatal("Cannot create bottomless pit jail")
	}
	err = posixPermDrop(bottomlessPit, conf.User, conf.Group)
	if err != nil {
		log.WithError(err).Fatal("Cannot drop permissions")
	}

	err = restrict(restrict_linux_seccomp,
		[]string{
			"@system-service",
			"~@chown",
			"~@clock",
			"~@cpu-emulation",
			"~@debug",
			"~@file-system",
			"~@keyring",
			"~@memlock",
			"~@module",
			"~@mount",
			"~@network-io",
			"~@privileged",
			"~@reboot",
			"~@sandbox",
			"~@setuid",
			"~@swap",
			/* @process */ "~execve", "~execveat", "~fork", "~kill",
		})
	if err != nil {
		log.Fatal(err)
	}

	err = restrict(restrict_openbsd_pledge, "stdio tty error", "")
	if err != nil {
		log.Fatal(err)
	}

	<-ctx.Done()
}

func main() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	var (
		flagConfig    string
		flagForkChild string
		flagVerbose   bool
	)

	flag.StringVar(&flagConfig, "config", "", "YAML configuration file")
	flag.StringVar(&flagForkChild, "fork-child", "", "Start a subprocess child")
	flag.BoolVar(&flagVerbose, "verbose", false, "Verbose logging")

	flag.Parse()

	if flagVerbose {
		log.SetLevel(log.DebugLevel)
	}

	conf, err := loadConfig(flagConfig)
	if err != nil {
		log.WithError(err).Fatal("Cannot parse YAML configuration")
	}

	switch flagForkChild {
	case "webserver":
		mainWebserver(conf)

	case "store":
		mainStore(conf)

	case "":
		mainMonitor(conf)

	default:
		log.WithField("fork-child", flagForkChild).Fatal("Unknown child process")
	}
}
