package main

import (
	"flag"
	"os"
	"os/signal"
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

func mainMonitor(conf Config) {
	storeRpcServer, storeRpcClient, err := socketpair()
	if err != nil {
		log.Fatal(err)
	}
	storeFdServer, storeFdClient, err := socketpair()
	if err != nil {
		log.Fatal(err)
	}

	procStore, err := forkChild("store", []*os.File{storeRpcServer, storeFdServer})
	if err != nil {
		log.Fatal(err)
	}

	procWebserver, err := forkChild("webserver", []*os.File{storeRpcClient, storeFdClient})
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
			/* @process */ "~execve", "~execveat", "~fork",
		})
	if err != nil {
		log.Fatal(err)
	}

	err = restrict(restrict_openbsd_pledge, "stdio tty proc error", "")
	if err != nil {
		log.Fatal(err)
	}

	sigintCh := make(chan os.Signal, 1)
	signal.Notify(sigintCh, unix.SIGINT)

	storeCh := make(chan struct{})
	procWait(storeCh, procStore)

	webserverCh := make(chan struct{})
	procWait(webserverCh, procWebserver)

	childProcs := []*os.Process{procStore, procWebserver}
	childWaits := []chan struct{}{storeCh, webserverCh}

	select {
	case <-sigintCh:
		log.Info("Main process receives SIGINT, shutting down")

	case <-storeCh:
		log.Error("The store subprocess has stopped, cleaning up")

	case <-webserverCh:
		log.Error("The web server subprocess has stopped, cleaning up")
	}

	for i, childProc := range childProcs {
		_ = childProc.Signal(unix.SIGINT)

		select {
		case <-childWaits[i]:
		case <-time.After(time.Second):
			_ = childProc.Kill()
		}
	}
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
