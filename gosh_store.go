package main

import (
	"os"
	"os/signal"

	log "github.com/sirupsen/logrus"
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
	log.WithField("config", conf.Store).Debug("Starting store child")

	err := ensureStoreDir(conf.Store.Path, conf.User, conf.Group)
	if err != nil {
		log.WithError(err).Fatal("Cannot prepare store directory")
	}

	err = posixPermDrop(conf.Store.Path, conf.User, conf.Group)
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
		log.Fatal(err)
	}

	err = restrict(restrict_openbsd_pledge,
		"stdio rpath wpath cpath flock unix sendfd recvfd error",
		"")
	if err != nil {
		log.Fatal(err)
	}

	store, err := NewStore("/", true)
	if err != nil {
		log.Fatal(err)
	}

	rpcConn, err := UnixConnFromFile(os.NewFile(3, ""))
	if err != nil {
		log.Fatal(err)
	}
	fdConn, err := UnixConnFromFile(os.NewFile(4, ""))
	if err != nil {
		log.Fatal(err)
	}

	rpcStore := NewStoreRpcServer(store, rpcConn, fdConn)

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	<-sigint

	err = rpcStore.Close()
	if err != nil {
		log.Fatal(err)
	}
}
