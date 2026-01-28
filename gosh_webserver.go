package main

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"

	"golang.org/x/sys/unix"
)

// mkListenSocket creates the socket for the web server to be bound to.
//
// Based on protocol ("tcp" or "unix") a TCP or Unix domain socket will be
// created for the given bound address. For an Unix domain socket, the socket
// will first be created for the current user (root?) with a restrict umask
// (which will be reset afterwards) and then chown'ed and chmod'ed to the
// configured settings.
func mkListenSocket(protocol, bound, unixChmod, unixOwner, unixGroup string) (*os.File, error) {
	switch protocol {
	case "tcp":
		ln, err := net.Listen("tcp", bound)
		if err != nil {
			return nil, err
		}
		return ln.(*net.TCPListener).File()

	case "unix":
		if _, stat := os.Stat(bound); stat == nil {
			if err := os.Remove(bound); err != nil {
				return nil, fmt.Errorf("cannot cleanup old Unix domain socket file %q: %v", bound, err)
			}
		}

		oldUmask := unix.Umask(unix.S_IXUSR | unix.S_IXGRP | unix.S_IWOTH | unix.S_IROTH | unix.S_IXOTH)
		defer unix.Umask(oldUmask)

		ln, err := net.Listen("unix", bound)
		if err != nil {
			return nil, err
		}

		ln.(*net.UnixListener).SetUnlinkOnClose(true)

		f, err := ln.(*net.UnixListener).File()
		if err != nil {
			return nil, err
		}

		uid, gid, err := uidGidForUserGroup(unixOwner, unixGroup)
		if err != nil {
			return nil, err
		}

		err = os.Chown(bound, uid, gid)
		if err != nil {
			return nil, err
		}

		unixChmodInt, err := strconv.ParseUint(unixChmod, 8, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot parse octal chmod %q: %v", unixChmod, err)
		}
		unixChmodMode := (fs.FileMode)(unixChmodInt)

		err = os.Chmod(bound, unixChmodMode)
		if err != nil {
			return nil, err
		}

		return f, nil

	default:
		return nil, fmt.Errorf("unsupported protocol %q", protocol)
	}
}

func mainWebserver(conf Config) {
	slog.Debug("Starting webserver child", slog.Any("config", conf.Webserver))

	rpcConn, err := unixConnFromFile(os.NewFile(3, ""))
	if err != nil {
		slog.Error("Failed to prepare store directory", slog.Any("error", err))
		os.Exit(1)
	}
	fdConn, err := unixConnFromFile(os.NewFile(4, ""))
	if err != nil {
		slog.Error("Failed to prepare store directory", slog.Any("error", err))
		os.Exit(1)
	}

	storeClient := NewStoreRpcClient(rpcConn, fdConn)

	indexTpl := ""
	if conf.Webserver.CustomIndex != "" {
		f, err := os.Open(conf.Webserver.CustomIndex)
		if err != nil {
			slog.Error("Failed to open custom index file", slog.Any("error", err))
			os.Exit(1)
		}

		indexTplRaw, err := io.ReadAll(f)
		if err != nil {
			slog.Error("Failed to read custom index file", slog.Any("error", err))
			os.Exit(1)
		}
		_ = f.Close()

		indexTpl = string(indexTplRaw)
	}

	for k, sfc := range conf.Webserver.StaticFiles {
		f, err := os.Open(sfc.Path)
		if err != nil {
			slog.Error("Failed to open static file",
				slog.String("file", sfc.Path), slog.Any("error", err))
			os.Exit(1)
		}

		sfc.data, err = io.ReadAll(f)
		if err != nil {
			slog.Error("Failed to read static file",
				slog.String("file", sfc.Path), slog.Any("error", err))
			os.Exit(1)
		}
		_ = f.Close()

		conf.Webserver.StaticFiles[k] = sfc
	}

	maxFilesize, err := ParseBytesize(conf.Webserver.ItemConfig.MaxSize)
	if err != nil {
		slog.Error("Failed to parse byte size", slog.Any("error", err))
		os.Exit(1)
	}

	mimeDrop := make(map[string]struct{})
	for _, key := range conf.Webserver.ItemConfig.MimeDrop {
		mimeDrop[key] = struct{}{}
	}

	fd, err := mkListenSocket(
		conf.Webserver.Listen.Protocol, conf.Webserver.Listen.Bound,
		conf.Webserver.UnixSocket.Chmod, conf.Webserver.UnixSocket.Owner, conf.Webserver.UnixSocket.Group)
	if err != nil {
		slog.Error("Failed to create listening socket", slog.Any("error", err))
		os.Exit(1)
	}

	bottomlessPit, err := os.MkdirTemp("", "gosh-webserver-chroot")
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
			"~@privileged",
			"~@reboot",
			"~@sandbox",
			"~@setuid",
			"~@swap",
			/* @process */ "~execve", "~execveat", "~fork", "~kill",
			/* @network-io */ "~bind", "~connect", "~listen",
		})
	if err != nil {
		slog.Error("Failed to apply seccomp-bpf filter", slog.Any("error", err))
		os.Exit(1)
	}

	err = restrict(restrict_openbsd_pledge,
		"stdio unix sendfd recvfd error",
		"")
	if err != nil {
		slog.Error("Failed to pledge", slog.Any("error", err))
		os.Exit(1)
	}

	server, err := NewServer(
		storeClient,
		maxFilesize,
		conf.Webserver.ItemConfig.MaxLifetime,
		conf.Webserver.Contact,
		mimeDrop,
		conf.Webserver.ItemConfig.MimeMap,
		conf.Webserver.UrlPrefix,
		indexTpl,
		conf.Webserver.StaticFiles,
	)
	if err != nil {
		slog.Error("Failed to create webserver", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() { _ = server.Close() }()

	sigintCh := make(chan os.Signal, 1)
	signal.Notify(sigintCh, unix.SIGINT)

	serverCh := make(chan struct{})
	go func() {
		switch conf.Webserver.Protocol {
		case "fcgi":
			err = server.ServeFcgi(fd)

		case "http":
			err = server.ServeHttpd(fd)

		default:
			err = fmt.Errorf("unsupported protocol %q", conf.Webserver.Protocol)
		}
		if err != nil && err != http.ErrServerClosed {
			slog.Error("Webserver failed to listen", slog.Any("error", err))
			os.Exit(1)
		}

		close(serverCh)
	}()

	select {
	case <-sigintCh:
		slog.Info("Stopping webserver")

	case <-serverCh:
		slog.Error("Webserver finished, shutting down")
	}
}
