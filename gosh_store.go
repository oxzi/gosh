package main

import (
	"os"
	"os/signal"

	log "github.com/sirupsen/logrus"
)

func mainStore(conf Config) {
	log.WithField("config", conf.Store).Debug("Starting store child")

	err := posixPermDrop(conf.Store.Path, conf.User, conf.Group)
	if err != nil {
		log.WithError(err).Fatal("Cannot drop permissions")
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
