package main

import (
	"flag"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"strings"
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
	socketFd    **os.File
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
	flag.StringVar(&listenAddr, "listen", ":8080", "Either an address for a HTTP server or a path prefixed with 'fcgi:' for a FastCGI unix socket")
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
	}
	if contactMail == "" {
		log.Fatal("Contact information must be set, see `--help`")
	}

	socketFd = new(*os.File)
	*socketFd = nil

	internal.Hardening(true, &storePath, &listenAddr, socketFd)
}

func serveHttpd(server *internal.Server) {
	webServer := &http.Server{
		Addr:    listenAddr,
		Handler: server,
	}

	log.WithField("listen", listenAddr).Info("Starting web server")

	if err := webServer.ListenAndServe(); err != http.ErrServerClosed {
		log.WithError(err).Fatal("Web server failed")
	}
}

func serveFcgi(server *internal.Server) {
	var (
		ln  net.Listener
		err error
	)

	socketAddr := listenAddr[len("fcgi:"):]

	if *socketFd != nil {
		ln, err = net.FileListener(*socketFd)
	} else {
		if _, stat := os.Stat(socketAddr); stat == nil {
			if err = os.Remove(socketAddr); err != nil {
				log.WithField("socket", socketAddr).WithError(err).Fatal("Cannot cleanup old socket file")
			}
		}

		ln, err = net.Listen("unix", socketAddr)
	}
	if err != nil {
		log.WithField("socket", socketAddr).WithError(err).Fatal("Cannot listen on unix socket")
	}

	log.WithField("socket", socketAddr).Info("Starting FastCGI server")

	if err := fcgi.Serve(ln, server); err != nil {
		log.WithError(err).Fatal("FastCGI server failed")
	}
}

func main() {
	server, err := internal.NewServer(
		storePath, maxFilesize, maxLifetime, contactMail, mimeMap)
	if err != nil {
		log.WithError(err).Fatal("Failed to start Store")
	}

	defer server.Close()

	if strings.HasPrefix(listenAddr, "fcgi:") {
		serveFcgi(server)
	} else {
		serveHttpd(server)
	}
}
