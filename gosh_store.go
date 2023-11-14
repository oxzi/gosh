package main

import (
	"log/slog"
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

// ensureStoreDir makes sure that a store directory exists and it holds the
// correct permissions.
func ensureStoreDir(path, username, groupname string) error {
	_, stat := os.Stat(path)
	if os.IsNotExist(stat) {
		err := os.Mkdir(path, 0700)
		if err != nil {
			return err
		}
	}

	err := os.Chmod(path, 0700)
	if err != nil {
		return err
	}

	uid, gid, err := uidGidForUserGroup(username, groupname)
	if err != nil {
		return err
	}
	err = os.Chown(path, uid, gid)
	if err != nil {
		return err
	}

	return nil
}

func mainStore(conf Config) {
	slog.Debug("Starting store child", slog.Any("config", conf.Store))

	var idGenerator func() (string, error)
	switch conf.Store.IdGenerator.Type {
	case "random":
		idGenerator = randomIdGenerator(conf.Store.IdGenerator.Length)

	default:
		slog.Error("Failed to configure an ID generator as the type is unknown",
			slog.String("type", conf.Store.IdGenerator.Type))
		os.Exit(1)
	}

	err := ensureStoreDir(conf.Store.Path, conf.User, conf.Group)
	if err != nil {
		slog.Error("Failed to prepare store directory", slog.Any("error", err))
		os.Exit(1)
	}

	err = posixPermDrop(conf.Store.Path, conf.User, conf.Group)
	if err != nil {
		slog.Error("Failed to drop permissions", slog.Any("error", err))
		os.Exit(1)
	}

	err = restrict(restrict_linux_seccomp,
		[]string{
			"@system-service",
			"~@chown",
			"~@clock",
			"~@cpu-emulation",
			"~@debug",
			"~@keyring",
			"~@memlock",
			"~@module",
			"~@mount",
			"~@privileged",
			"~@reboot",
			"~@sandbox",
			"~@setuid",
			"~@swap",
			/* @process */ "~execve", "~execveat", "~fork", "~kill",
			/* @network-io */ "~bind", "~connect", "~listen",
		})
	if err != nil {
		slog.Error("Failed to apply seccomp-bpf filter", slog.Any("error", err))
		os.Exit(1)
	}

	err = restrict(restrict_openbsd_pledge,
		"stdio rpath wpath cpath flock unix sendfd recvfd error",
		"")
	if err != nil {
		slog.Error("Failed to pledge", slog.Any("error", err))
		os.Exit(1)
	}

	store, err := NewStore("/", idGenerator, true)
	if err != nil {
		slog.Error("Failed to create store", slog.Any("error", err))
		os.Exit(1)
	}

	rpcConn, err := unixConnFromFile(os.NewFile(3, ""))
	if err != nil {
		slog.Error("Failed to create Unix Domain Socket from FD", slog.Any("error", err))
		os.Exit(1)
	}
	fdConn, err := unixConnFromFile(os.NewFile(4, ""))
	if err != nil {
		slog.Error("Failed to create Unix Domain Socket from FD", slog.Any("error", err))
		os.Exit(1)
	}

	rpcStore := NewStoreRpcServer(store, rpcConn, fdConn)

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, unix.SIGINT)
	<-sigint

	err = rpcStore.Close()
	if err != nil {
		slog.Error("Failed to close RPC Store", slog.Any("error", err))
		os.Exit(1)
	}
}
