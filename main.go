package main

import (
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

func main() {
	server, err := NewServer("store", 10*1024*1024, 10*time.Minute, "foo@bar.buz")
	if err != nil {
		log.WithError(err).Fatal("Failed to start Store")
	}

	http.ListenAndServe(":8080", server)

	if err := server.Close(); err != nil {
		log.WithError(err).Fatal("Closing errored")
	}
}
