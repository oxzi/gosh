package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

// StaticFileConfig describes a static_files from the YAML and holds its data.
type StaticFileConfig struct {
	Path string `yaml:"path"`
	Mime string `yaml:"mime"`

	data []byte
}

// Config is the struct representation of gosh's YAML configuration file.
//
// For each field's meaning, please consider the gosh.yml file in this
// repository as it serves both as an example as well as documentation and
// otherwise the documentation will diverge anyways.
type Config struct {
	User  string
	Group string

	Store struct {
		Path string

		IdGenerator struct {
			Type   string `yaml:"type"`
			Length int    `yaml:"length"`
			File   string `yaml:"file"`
		} `yaml:"id_generator"`
	}

	Webserver struct {
		Listen struct {
			Protocol string
			Bound    string
		}

		UnixSocket struct {
			Chmod string
			Owner string
			Group string
		} `yaml:"unix_socket"`

		Protocol string

		UrlPrefix string `yaml:"url_prefix"`

		CustomIndex string `yaml:"custom_index"`

		StaticFiles map[string]StaticFileConfig `yaml:"static_files"`

		ItemConfig struct {
			MaxSize     string        `yaml:"max_size"`
			MaxLifetime time.Duration `yaml:"max_lifetime"`

			MimeDrop []string          `yaml:"mime_drop"`
			MimeMap  map[string]string `yaml:"mime_map"`
		} `yaml:"item_config"`

		Contact string
	}
}

// loadConfig loads a Config from a given YAML configuration file at the path.
func loadConfig(path string) (Config, error) {
	var conf Config

	f, err := os.Open(path)
	if err != nil {
		return conf, err
	}
	defer func() { _ = f.Close() }()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&conf)
	return conf, err
}

func mainMonitor(conf Config) {
	storeRpcServer, storeRpcClient, err := socketpair()
	if err != nil {
		slog.Error("Failed to create socketpair", slog.Any("error", err))
		os.Exit(1)
	}
	storeFdServer, storeFdClient, err := socketpair()
	if err != nil {
		slog.Error("Failed to create socketpair", slog.Any("error", err))
		os.Exit(1)
	}

	procStore, err := forkChild("store", []*os.File{storeRpcServer, storeFdServer})
	if err != nil {
		slog.Error("Failed to fork off child", slog.Any("error", err), slog.String("child", "store"))
		os.Exit(1)
	}

	procWebserver, err := forkChild("webserver", []*os.File{storeRpcClient, storeFdClient})
	if err != nil {
		slog.Error("Failed to fork off child", slog.Any("error", err), slog.String("child", "webserver"))
		os.Exit(1)
	}

	bottomlessPit, err := os.MkdirTemp("", "gosh-monitor-chroot")
	if err != nil {
		slog.Error("Failed to create bottomless pit jail", slog.Any("error", err))
		os.Exit(1)
	}
	err = posixPermDrop(bottomlessPit, conf.User, conf.Group)
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
			"~@network-io",
			"~@privileged",
			"~@reboot",
			"~@sandbox",
			"~@setuid",
			"~@swap",
			/* @process */ "~execve", "~execveat", "~fork",
		})
	if err != nil {
		slog.Error("Failed to apply seccomp-bpf filter", slog.Any("error", err))
		os.Exit(1)
	}

	err = restrict(restrict_openbsd_pledge, "stdio tty proc error", "")
	if err != nil {
		slog.Error("Failed to pledge", slog.Any("error", err))
		os.Exit(1)
	}

	sigintCh := make(chan os.Signal, 1)
	signal.Notify(sigintCh, unix.SIGINT)

	storeCh := make(chan struct{})
	procWait(storeCh, procStore)

	webserverCh := make(chan struct{})
	procWait(webserverCh, procWebserver)

	childProcs := []*os.Process{procStore, procWebserver}
	childWaits := []chan struct{}{storeCh, webserverCh}

	select {
	case <-sigintCh:
		slog.Info("Main process receives SIGINT, shutting down")

	case <-storeCh:
		slog.Error("The store subprocess has stopped, cleaning up")

	case <-webserverCh:
		slog.Error("The web server subprocess has stopped, cleaning up")
	}

	for i, childProc := range childProcs {
		_ = childProc.Signal(unix.SIGINT)

		select {
		case <-childWaits[i]:
		case <-time.After(time.Second):
			_ = childProc.Kill()
		}
	}
}

// configureLogger sets the default logger with an optional debug log level and
// JSON encoded output, useful for the forked off childs.
func configureLogger(debug, jsonOutput bool) {
	loggerLevel := new(slog.LevelVar)
	if debug {
		loggerLevel.Set(slog.LevelDebug)
	}

	handlerOpts := &slog.HandlerOptions{Level: loggerLevel}

	var logger *slog.Logger
	if jsonOutput {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, handlerOpts))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, handlerOpts))
	}

	slog.SetDefault(logger)
}

func main() {
	var (
		flagConfig    string
		flagForkChild string
		flagVerbose   bool
	)

	flag.StringVar(&flagConfig, "config", "", "YAML configuration file")
	flag.StringVar(&flagForkChild, "fork-child", "", "Start a subprocess child")
	flag.BoolVar(&flagVerbose, "verbose", false, "Verbose logging")

	flag.Parse()

	configureLogger(flagVerbose, flagForkChild != "")

	conf, err := loadConfig(flagConfig)
	if err != nil {
		slog.Error("Failed to parse YAML configuration", slog.Any("error", err))
		os.Exit(1)
	}

	switch flagForkChild {
	case "webserver":
		mainWebserver(conf)

	case "store":
		mainStore(conf)

	case "":
		mainMonitor(conf)

	default:
		slog.Error("Unknown child process identifier", slog.String("name", flagForkChild))
		os.Exit(1)
	}
}
