package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/oxzi/gosh/internal"
)

var (
	storePath   string
	maxFilesize int64
	maxLifetime time.Duration
	contactMail string
	mimeMap     internal.MimeMap
	listenAddr  string
	verbose     bool
)

func init() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	var (
		maxLifetimeStr string
		maxFilesizeStr string
		mimeMapStr     string
	)

	flag.StringVar(&storePath, "store", "", "Path to the store")
	flag.StringVar(&maxFilesizeStr, "max-filesize", "10MiB", "Maximum file size in bytes")
	flag.StringVar(&maxLifetimeStr, "max-lifetime", "24h", "Maximum lifetime")
	flag.StringVar(&contactMail, "contact", "", "Contact E-Mail for abuses")
	flag.StringVar(&mimeMapStr, "mimemap", "", "MimeMap to substitute/drop MIMEs")
	flag.StringVar(&listenAddr, "listen", ":8080", "Listen address for the HTTP server")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging")

	flag.Parse()

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	if lt, err := internal.ParseDuration(maxLifetimeStr); err != nil {
		log.WithError(err).Fatal("Failed to parse lifetime")
	} else {
		maxLifetime = lt
	}

	if bs, err := internal.ParseBytesize(maxFilesizeStr); err != nil {
		log.WithError(err).Fatal("Failed to parse byte size")
	} else {
		maxFilesize = bs
	}

	if mimeMapStr == "" {
		mimeMap = make(internal.MimeMap)
	} else {
		if f, err := os.Open(mimeMapStr); err != nil {
			log.WithError(err).Fatal("Failed to open MimeMap")
		} else if mm, err := internal.NewMimeMap(f); err != nil {
			log.WithError(err).Fatal("Failed to parse MimeMap")
		} else {
			f.Close()
			mimeMap = mm
		}
	}

	if storePath == "" {
		log.Fatal("Store Path must be set, see `--help`")
	} else if contactMail == "" {
		log.Fatal("Contact information must be set, see `--help`")
	}
}

func webserver(server *internal.Server) {
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

	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second)
	if err := webServer.Shutdown(ctx); err != nil {
		log.WithError(err).Fatal("Failed to shutdown web server")
	}
	ctxCancel()
}

func main() {
	server, err := internal.NewServer(
		storePath, maxFilesize, maxLifetime, contactMail, mimeMap)
	if err != nil {
		log.WithError(err).Fatal("Failed to start Store")
	}

	webserver(server)

	if err := server.Close(); err != nil {
		log.WithError(err).Fatal("Closing errored")
	}
}
