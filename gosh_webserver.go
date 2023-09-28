package main

import (
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"time"

	"golang.org/x/sys/unix"

	log "github.com/sirupsen/logrus"
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

		unixOwnerStruct, err := user.Lookup(unixOwner)
		if err != nil {
			return nil, err
		}
		unixOwnerId, err := strconv.ParseInt(unixOwnerStruct.Uid, 10, 64)
		if err != nil {
			return nil, err
		}
		unixGroupStruct, err := user.LookupGroup(unixGroup)
		if err != nil {
			return nil, err
		}
		unixGroupId, err := strconv.ParseInt(unixGroupStruct.Gid, 10, 64)
		if err != nil {
			return nil, err
		}

		err = os.Chown(bound, int(unixOwnerId), int(unixGroupId))
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

func mainWebserver() {
	log.Info("Starting web server child")

	rpcConn, err := UnixConnFromFile(os.NewFile(3, ""))
	if err != nil {
		log.Fatal(err)
	}
	fdConn, err := UnixConnFromFile(os.NewFile(4, ""))
	if err != nil {
		log.Fatal(err)
	}

	storeClient := NewStoreRpcClient(rpcConn, fdConn)

	// XXX replace with configuration
	/*
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
	*/
	var (
		protocol  string = "tcp"
		bound     string = ":8080"
		unixChmod string = "0600"
		unixOwner string = "www"
		unixGroup string = "www"

		maxFilesize int64         = 10 * 1024 * 1024
		maxLifetime time.Duration = 24 * time.Hour
		contactMail string        = "nobody@example.com"
		mimeMap     MimeMap       = make(MimeMap)
		urlPrefix   string        = ""

		fcgiServer bool = false
	)

	fd, err := mkListenSocket(protocol, bound, unixChmod, unixOwner, unixGroup)
	if err != nil {
		log.WithError(err).Fatal("Cannot create socket to be bound to")
	}

	server, err := NewServer(storeClient, maxFilesize, maxLifetime, contactMail, mimeMap, urlPrefix)
	if err != nil {
		log.WithError(err).Fatal("Cannot create web server")
	}
	defer server.Close()

	if fcgiServer {
		err = server.ListenFcgi(fd)
	} else {
		err = server.ListenHttpd(fd)
	}
	if err != nil && err != http.ErrServerClosed {
		log.WithError(err).Error("Web server failed to listen")
	}
}
