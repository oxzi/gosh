//go:build aix || linux || darwin || dragonfly || freebsd || openbsd || netbsd || solaris

// This file contains some common code for multiple Unix platforms - or might it
// be POSIX? I don't know. However, it should be feasible for BSDs and Linux.

package internal

import (
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	syscall "golang.org/x/sys/unix"
)

// normalizePaths to be used later on in the code.
func (opts *HardeningOpts) normalizePaths() {
	pathObjs := []*string{opts.StoreDir}
	if opts.ListenUnixAddr != nil {
		pathObjs = append(pathObjs, opts.ListenUnixAddr)
	}
	if opts.MimeMapFile != nil {
		pathObjs = append(pathObjs, opts.MimeMapFile)
	}

	for _, pathObj := range pathObjs {
		absPathObj, err := filepath.Abs(*pathObj)
		if err != nil {
			log.WithError(err).WithField("path", *pathObj).Fatal("Cannot create an absolute path")
		}
		*pathObj = absPathObj
	}
}

// getUidGid returns the requested ChangeUser's UID and GID.
func (opts *HardeningOpts) getUidGid() (uid int, gid int) {
	sysUser, err := user.Lookup(*(opts.ChangeUser))
	if err != nil {
		log.WithField("user", *(opts.ChangeUser)).WithError(err).Fatal("Cannot find user")
	}
	uid64, _ := strconv.ParseInt(sysUser.Uid, 10, 64)
	gid64, _ := strconv.ParseInt(sysUser.Gid, 10, 64)
	return int(uid64), int(gid64)
}

// mkListenSocket populates ListenSocket with either a TCP or an Unix domain
// socket's file descriptor.
func (opts *HardeningOpts) mkListenSocket() {
	switch {
	case opts.ListenTcpAddr != nil:
		socketAddr := *(opts.ListenTcpAddr)

		ln, err := net.Listen("tcp", socketAddr)
		if err != nil {
			log.WithError(err).Fatal("Cannot listen on TCP")
		}

		opts.ListenSocket, err = ln.(*net.TCPListener).File()
		if err != nil {
			log.WithError(err).Fatal("Cannot get TCP listener's file descriptor")
		}
		log.WithField("listen", socketAddr).Info("Created TCP listener")

	case opts.ListenUnixAddr != nil:
		socketAddr := *(opts.ListenUnixAddr)

		if _, stat := os.Stat(socketAddr); stat == nil {
			if err := os.Remove(socketAddr); err != nil {
				log.WithError(err).Fatal("Cannot cleanup old Unix domain socket file")
			}
		}

		oldUmask := syscall.Umask(syscall.S_IXUSR | syscall.S_IXGRP | syscall.S_IWOTH | syscall.S_IROTH | syscall.S_IXOTH)
		defer syscall.Umask(oldUmask)

		ln, err := net.Listen("unix", socketAddr)
		if err != nil {
			log.WithError(err).Fatal("Cannot listen on Unix domain socket")
		}

		ln.(*net.UnixListener).SetUnlinkOnClose(true)

		opts.ListenSocket, err = ln.(*net.UnixListener).File()
		if err != nil {
			log.WithError(err).Fatal("Cannot get Unix domain socket's file descriptor")
		}
		log.WithField("listen", socketAddr).Info("Created Unix domain socket listener")

		if opts.ChangeUser != nil {
			uid, gid := opts.getUidGid()
			err = os.Chown(socketAddr, uid, gid)
			if err != nil {
				log.WithError(err).Fatal("Cannot chown Unix domain socket")
			}
			log.WithFields(log.Fields{
				"listen": socketAddr,
				"user":   *(opts.ChangeUser),
			}).Debug("Changed Unix domain socket's ownership")
		}

	}
}

// chroot this application into a common parent directory.
//
// HardeningOpts.normalizePaths MUST be called first.
func (opts *HardeningOpts) chroot() {
	const sep = string(os.PathSeparator)

	dirs := [][]string{strings.Split(filepath.Dir(*(opts.StoreDir)), sep)}
	if opts.ListenUnixAddr != nil {
		dirs = append(dirs, strings.Split(filepath.Dir(*(opts.ListenUnixAddr)), sep))
	}
	if opts.MimeMapFile != nil {
		dirs = append(dirs, strings.Split(filepath.Dir(*(opts.MimeMapFile)), sep))
	}

	rootParts := []string{}
	for i := range dirs[0] {
		part, ok := dirs[0][i], true
		for j := range dirs {
			if len(dirs[j]) <= i || dirs[j][i] != part {
				ok = false
				break
			}
		}

		if !ok {
			break
		}
		rootParts = append(rootParts, part)
	}

	if len(rootParts) <= 1 {
		log.WithField("dirs", dirs).Warn("Cannot find common directory below root, no chroot!")
		return
	}
	rootDir := strings.Join(rootParts, sep)

	if err := syscall.Chroot(rootDir); err != nil {
		log.WithError(err).WithField("chroot", rootDir).Fatal("Cannot chroot")
	}
	if err := syscall.Chdir(sep); err != nil {
		log.WithError(err).WithField("chroot", rootDir).Fatal("Cannot chdir after chroot")
	}

	*(opts.StoreDir) = (*(opts.StoreDir))[len(rootDir):]
	if opts.ListenUnixAddr != nil {
		*(opts.ListenUnixAddr) = (*(opts.ListenUnixAddr))[len(rootDir):]
	}
	if opts.MimeMapFile != nil {
		*(opts.MimeMapFile) = (*(opts.MimeMapFile))[len(rootDir):]
	}

	log.WithField("chroot", rootDir).Debug("Switched to chroot environment")
}

// changeUser to the configured ChangeUser.
//
// The UID and GID MUST be passed as they aren't available after chrooting.
func (opts *HardeningOpts) changeUser(uid, gid int) {
	logFields := log.Fields{
		"user": *(opts.ChangeUser),
		"uid":  uid,
		"gid":  gid,
	}

	if err := syscall.Setgroups([]int{gid}); err != nil {
		log.WithError(err).WithFields(logFields).Fatal("Cannot setgroups")
	}
	if err := syscall.Setresgid(gid, gid, gid); err != nil {
		log.WithError(err).WithFields(logFields).Fatal("Cannot setresgid")
	}
	if err := syscall.Setresuid(uid, uid, uid); err != nil {
		log.WithError(err).WithFields(logFields).Fatal("Cannot setresuid")
	}

	log.WithFields(logFields).Debug("Changed UID and GID")
}

// applyUnix hardening.
func (opts *HardeningOpts) applyUnix() {
	opts.normalizePaths()
	opts.mkListenSocket()

	if opts.ChangeUser != nil {
		uid, gid := opts.getUidGid()

		opts.chroot()
		opts.changeUser(uid, gid)
	}
}
