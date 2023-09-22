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
)

var (
	storePath   string
	maxFilesize int64
	maxLifetime time.Duration
	contactMail string
	mimeMap     MimeMap
	urlPrefix   string
	fcgiServer  bool
	socketFd    *os.File
)

func serveFcgi(server *Server) {
	ln, err := net.FileListener(socketFd)
	if err != nil {
		log.WithError(err).Fatal("Cannot listen on socket")
	}

	log.Info("Starting FastCGI server")
	if err := fcgi.Serve(ln, server); err != nil {
		log.WithError(err).Fatal("FastCGI server failed")
	}
}

func serveHttpd(server *Server) {
	webServer := &http.Server{
		Handler: server,
	}
	ln, err := net.FileListener(socketFd)
	if err != nil {
		log.WithError(err).Fatal("Cannot listen on socket")
	}

	log.Info("Starting web server")
	if err := webServer.Serve(ln); err != http.ErrServerClosed {
		log.WithError(err).Fatal("Web server failed")
	}
}

func main() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	var (
		maxLifetimeStr string
		maxFilesizeStr string
		mimeMapStr     string
		listenAddr     string
		user           string
		verbose        bool
	)

	flag.StringVar(&storePath, "store", "", "Path to the store")
	flag.StringVar(&maxFilesizeStr, "max-filesize", "10MiB", "Maximum file size in bytes")
	flag.StringVar(&maxLifetimeStr, "max-lifetime", "24h", "Maximum lifetime")
	flag.StringVar(&contactMail, "contact", "", "Contact E-Mail for abuses")
	flag.StringVar(&mimeMapStr, "mimemap", "", "MimeMap to substitute/drop MIMEs")
	flag.StringVar(&listenAddr, "listen", ":8080", "Either a TCP listen address or an Unix domain socket")
	flag.StringVar(&urlPrefix, "url-prefix", "", "Prefix in URL to be used, e.g., /gosh")
	flag.BoolVar(&fcgiServer, "fcgi", false, "Serve a FastCGI server instead of a HTTP server")
	flag.StringVar(&user, "user", "", "User to drop privileges to, also create a chroot - requires root permissions")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging")

	flag.Parse()

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	if lt, err := ParseDuration(maxLifetimeStr); err != nil {
		log.WithError(err).Fatal("Failed to parse lifetime")
	} else {
		maxLifetime = lt
	}

	if bs, err := ParseBytesize(maxFilesizeStr); err != nil {
		log.WithError(err).Fatal("Failed to parse byte size")
	} else {
		maxFilesize = bs
	}

	if storePath == "" {
		log.Fatal("Store Path must be set, see `--help`")
	}
	if contactMail == "" {
		log.Fatal("Contact information must be set, see `--help`")
	}

	hardeningOpts := &HardeningOpts{
		StoreDir: &storePath,
	}

	if strings.HasPrefix(listenAddr, ".") || strings.HasPrefix(listenAddr, "/") {
		hardeningOpts.ListenUnixAddr = &listenAddr
	} else {
		hardeningOpts.ListenTcpAddr = &listenAddr
	}
	if user != "" {
		hardeningOpts.ChangeUser = &user
	}
	if mimeMapStr != "" {
		hardeningOpts.MimeMapFile = &mimeMapStr
	}

	hardeningOpts.Apply()

	socketFd = hardeningOpts.ListenSocket

	if mimeMapStr == "" {
		mimeMap = make(MimeMap)
	} else {
		if f, err := os.Open(mimeMapStr); err != nil {
			log.WithError(err).Fatal("Failed to open MimeMap")
		} else if mm, err := NewMimeMap(f); err != nil {
			log.WithError(err).Fatal("Failed to parse MimeMap")
		} else {
			f.Close()
			mimeMap = mm
		}
	}

	server, err := NewServer(storePath, maxFilesize, maxLifetime, contactMail, mimeMap, urlPrefix)
	if err != nil {
		log.WithError(err).Fatal("Failed to start Store")
	}

	defer server.Close()

	if fcgiServer {
		serveFcgi(server)
	} else {
		serveHttpd(server)
	}
}
