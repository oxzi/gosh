package main

import (
	"bufio"
	"flag"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"os/exec"
	"os/signal"
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

func mainStore(storePath string) {
	log.WithField("store", storePath).Info("Starting store child")

	store, err := NewStore(storePath, true)
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

func main() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	var (
		forkStore      bool
		maxLifetimeStr string
		maxFilesizeStr string
		mimeMapStr     string
		listenAddr     string
		user           string
		verbose        bool
	)

	log.WithField("args", os.Args).Info("args")

	flag.StringVar(&storePath, "store", "", "Path to the store")
	flag.BoolVar(&forkStore, "fork-store", false, "Start the store sub")
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

	if forkStore {
		mainStore(storePath)
		return
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

	logParent, logStore, err := Socketpair()
	if err != nil {
		log.Fatal(err)
	}
	rpcParent, rpcStore, err := Socketpair()
	if err != nil {
		log.Fatal(err)
	}
	fdParent, fdStore, err := Socketpair()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		scanner := bufio.NewScanner(logParent)
		for scanner.Scan() {
			log.Printf("[store] %s", scanner.Text())
			if err := scanner.Err(); err != nil {
				log.Printf("scanner failed: %v", err)
			}
		}
	}()

	cmd := &exec.Cmd{
		Path: os.Args[0],
		Args: append(os.Args, "-fork-store"),

		Env: []string{},

		Stdin:      nil,
		Stdout:     logStore,
		Stderr:     logStore,
		ExtraFiles: []*os.File{rpcStore, fdStore},
	}
	err = cmd.Start()
	if err != nil {
		log.Fatal(err)
	}

	rpcConn, err := UnixConnFromFile(rpcParent)
	if err != nil {
		log.Fatal(err)
	}
	fdConn, err := UnixConnFromFile(fdParent)
	if err != nil {
		log.Fatal(err)
	}

	storeClient := NewStoreRpcClient(rpcConn, fdConn)

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

	server, err := NewServer(storeClient, maxFilesize, maxLifetime, contactMail, mimeMap, urlPrefix)
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
