package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"

	"github.com/oxzi/gosh/internal"
	"github.com/spf13/viper"
)

var (
	configPath  string
	storePath   string
	maxFilesize int64
	maxLifetime time.Duration
	contactMail string
	mimeMap     internal.MimeMap
	listenAddr  string
	verbose     bool
	encrypt     bool
	chunkSize   uint64
)

func init() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	var (
		maxLifetimeStr string
		maxFilesizeStr string
		mimeMapStr     string
		chunkSizeStr   string
	)

	flag.StringVar(&configPath, "config", "", "Path to an alternative config file")
	flag.StringVar(&storePath, "store", "", "Path to the store")
	flag.StringVar(&maxFilesizeStr, "max-filesize", "10MiB", "Maximum file size in bytes")
	flag.StringVar(&maxLifetimeStr, "max-lifetime", "24h", "Maximum lifetime")
	flag.StringVar(&contactMail, "contact", "", "Contact E-Mail for abuses")
	flag.StringVar(&mimeMapStr, "mimemap", "", "MimeMap to substitute/drop MIMEs")
	flag.StringVar(&listenAddr, "listen", ":8080", "Listen address for the HTTP server")
	flag.StringVar(&chunkSizeStr, "chunk-size", "1MiB", "Size of chunks for large files. Only relevant if encryption is switched on")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging")
	flag.BoolVar(&encrypt, "encrypt", false, "Encrypt stored data")
	flag.Parse()

	err := viper.BindPFlags(flag.CommandLine)
	if err != nil {
		log.WithError(err).Fatal("")
	}

	viper.SetDefault("max-filesize", "10MiB")
	viper.SetDefault("max-lifetime", "24h")
	viper.SetDefault("listen", ":8080")
	viper.SetDefault("chunk-size", "1MiB")
	viper.SetDefault("verbose", false)
	viper.SetDefault("encrypt", false)

	viper.SetConfigName("goshd")
	viper.SetConfigType("toml")
	viper.AddConfigPath("/etc/")
	viper.AddConfigPath(".")

	if viper.GetString("config") != "" {
		viper.SetConfigFile(viper.GetString("config"))
	}

	err = viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.WithError(err).Fatal("Error reading config file.")
		}
	}

	if viper.GetBool("verbose") {
		log.SetLevel(log.DebugLevel)
		viper.Debug()
	}

	if viper.GetString("store") == "" {
		log.Fatal("Store Path must be set, see `--help`")
	} else {
		storePath = viper.GetString("store")
	}

	if bs, err := internal.ParseBytesize(viper.GetString("max-filesize")); err != nil {
		log.WithError(err).Fatal("Failed to parse byte size")
	} else {
		maxFilesize = bs
	}

	if lt, err := internal.ParseDuration(viper.GetString("max-lifetime")); err != nil {
		log.WithError(err).Fatal("Failed to parse lifetime")
	} else {
		maxLifetime = lt
	}

	if viper.GetString("contact") == "" {
		log.Fatal("Contact information must be set, see `--help`")
	} else {
		contactMail = viper.GetString("contact")
	}

	if viper.GetString("mimemap") == "" {
		mimeMap = make(internal.MimeMap)
	} else {
		if f, err := os.Open(viper.GetString("mimemap")); err != nil {
			log.WithError(err).Fatal("Failed to open MimeMap")
		} else if mm, err := internal.NewMimeMap(f); err != nil {
			log.WithError(err).Fatal("Failed to parse MimeMap")
		} else {
			f.Close()
			mimeMap = mm
		}
	}

	listenAddr = viper.GetString("listen")

	encrypt = viper.GetBool("encrypt")

	if cs, err := internal.ParseBytesize(viper.GetString("chunk-size")); err != nil {
		log.WithError(err).Fatal("Failed to parse byte size")
	} else {
		chunkSize = uint64(cs)
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
		storePath, maxFilesize, maxLifetime, contactMail, mimeMap, encrypt, chunkSize)
	if err != nil {
		log.WithError(err).Fatal("Failed to start Store")
	}

	webserver(server)

	if err := server.Close(); err != nil {
		log.WithError(err).Fatal("Closing errored")
	}
}
