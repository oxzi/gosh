package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/geistesk/gosh"
)

var (
	storePath   string
	maxFilesize int64
	maxLifetime time.Duration
	contactMail string
	listenAddr  string
	verbose     bool
)

func init() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	var maxLifetimeStr string

	flag.StringVar(&storePath, "store", "", "Path to the store")
	flag.Int64Var(&maxFilesize, "max-filesize", 50*1024*1024, "Maximum file size in bytes")
	flag.StringVar(&maxLifetimeStr, "max-lifetime", "24h", "Maximum lifetime")
	flag.StringVar(&contactMail, "contact", "", "Contact E-Mail for abuses")
	flag.StringVar(&listenAddr, "listen", ":8080", "Listen address for the HTTP server")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging")

	flag.Parse()

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	if lt, err := time.ParseDuration(maxLifetimeStr); err != nil {
		log.WithError(err).Fatal("Failed to parse lifetime")
	} else {
		maxLifetime = lt
	}

	if storePath == "" {
		log.Fatal("Store Path must be set, see `--help`")
	} else if contactMail == "" {
		log.Fatal("Contact information must be set, see `--help`")
	}
}

func webserver(server *gosh.Server) {
	webServer := &http.Server{
		Addr:    listenAddr,
		Handler: server,
	}

	go func() {
		log.WithField("listen", listenAddr).Info("Starting web server")

		if err := webServer.ListenAndServe(); err != http.ErrServerClosed {
			log.WithError(err).Fatal("Web server errored")
		}
	}()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt)

	<-stopChan
	log.Info("Closing web server")

	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	if err := webServer.Shutdown(ctx); err != nil {
		log.WithError(err).Fatal("Failed to shutdown web server")
	}
}

func main() {
	server, err := gosh.NewServer(storePath, maxFilesize, maxLifetime, contactMail)
	if err != nil {
		log.WithError(err).Fatal("Failed to start Store")
	}

	webserver(server)

	if err := server.Close(); err != nil {
		log.WithError(err).Fatal("Closing errored")
	}
}
