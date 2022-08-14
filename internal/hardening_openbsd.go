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

const wwwUser = "www"

func hardeningGetUser() (int, int) {
	sysUser, err := user.Lookup(wwwUser)
	if err != nil {
		log.WithField("user", wwwUser).WithError(err).Fatal("Cannot find user")
	}
	uid, _ := strconv.ParseInt(sysUser.Uid, 10, 32)
	gid, _ := strconv.ParseInt(sysUser.Gid, 10, 32)
	return int(uid), int(gid)
}

func hardeningNormalizePath(storePath, listenAddr *string) {
	var err error

	if *storePath, err = filepath.Abs(*storePath); err != nil {
		log.WithError(err).Fatal("Cannot get an absolute storePath")
	}

	if strings.HasPrefix(*listenAddr, "fcgi:") {
		if la, err := filepath.Abs((*listenAddr)[len("fcgi:"):]); err != nil {
			log.WithError(err).Fatal("Cannot get an absolute listenAddr")
		} else {
			*listenAddr = "fcgi:" + la
		}
	}
}

func hardeningSocketFd(listenAddr string, socketFd **os.File, uid, gid int) {
	socketAddr := listenAddr[len("fcgi:"):]

	if _, stat := os.Stat(socketAddr); stat == nil {
		if err := os.Remove(socketAddr); err != nil {
			log.WithField("socket", socketAddr).WithError(err).Fatal("Cannot cleanup old socket file")
		}
	}

	oldUmask := syscall.Umask(syscall.S_IXUSR | syscall.S_IXGRP | syscall.S_IWOTH | syscall.S_IROTH | syscall.S_IXOTH)
	defer syscall.Umask(oldUmask)

	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketAddr, Net: "unix"})
	if err != nil {
		log.WithError(err).Fatal("Cannot listen on socket")
	}

	ln.SetUnlinkOnClose(true)

	*socketFd, err = ln.File()
	if err != nil {
		log.WithError(err).Fatal("Cannot get socket's file descriptor")
	}

	err = os.Chown(socketAddr, uid, gid)
	if err != nil {
		log.WithField("uid", uid).WithField("gid", gid).WithError(err).Fatal("Cannot chown socket")
	}

	log.WithField("socket", socketAddr).Debug("Created socket and chown'ed it for " + wwwUser)
}

func hardeningChroot(storePath, listenAddr *string) {
	dirs := [][]string{strings.Split(filepath.Clean(*storePath), "/")}
	if strings.HasPrefix(*listenAddr, "fcgi:") {
		dirs = append(dirs, strings.Split(filepath.Dir(filepath.Clean((*listenAddr)[len("fcgi:"):])), "/"))
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

	if len(rootParts) == 0 {
		log.WithField("dirs", dirs).Warn("Cannot find common parent directory, no chroot!")
		return
	}
	rootDir := strings.Join(rootParts, "/")

	if err := syscall.Chroot(rootDir); err != nil {
		log.WithError(err).Fatal("Cannot chroot")
	}
	if err := syscall.Chdir("/"); err != nil {
		log.WithError(err).Fatal("Cannot chdir after chroot")
	}

	*storePath = (*storePath)[len(rootDir):]
	if strings.HasPrefix(*listenAddr, "fcgi:") {
		*listenAddr = "fcgi:" + (*listenAddr)[len("fcgi:"+rootDir):]
	}

	log.WithField("root", rootDir).Info("Applied chroot")
}

func hardeningDropPrivs(uid, gid int) {
	if err := syscall.Setgroups([]int{gid}); err != nil {
		log.WithError(err).Fatal("Cannot setgroups")
	}

	if err := syscall.Setresgid(gid, gid, gid); err != nil {
		log.WithError(err).Fatal("Cannot setresgid")
	}

	if err := syscall.Setresuid(uid, uid, uid); err != nil {
		log.WithError(err).Fatal("Cannot setresuid")
	}

	log.Debug("Changed UID/GID to " + wwwUser)
}

func hardeningUnveil(storePath, listenAddr *string) {
	if err := syscall.Unveil(*storePath, "rwc"); err != nil {
		log.WithError(err).Fatal("Cannot unveil storePath")
	}

	if strings.HasPrefix(*listenAddr, "fcgi:") {
		if err := syscall.Unveil((*listenAddr)[len("fcgi:"):], "rw"); err != nil {
			log.WithError(err).Fatal("Cannot unveil listenAddr")
		}
	}

	if err := syscall.UnveilBlock(); err != nil {
		log.WithError(err).Fatal("Cannot unveil(NULL, NULL)")
	}
}

// Hardening will active some platform-specific hardening.
func Hardening(useNetwork bool, storePath, listenAddr *string, socketFd **os.File) {
	hardeningNormalizePath(storePath, listenAddr)

	uid, gid := hardeningGetUser()

	if strings.HasPrefix(*listenAddr, "fcgi:") {
		hardeningSocketFd(*listenAddr, socketFd, uid, gid)
	}

	hardeningChroot(storePath, listenAddr)

	hardeningDropPrivs(uid, gid)

	hardeningUnveil(storePath, listenAddr)

	// TODO differentiate between fcgi (unix) and http (tcp) and honor useNetwork
	if err := syscall.PledgePromises("stdio rpath wpath cpath flock unix proc"); err != nil {
		log.WithError(err).Fatal("Cannot pledge")
	}
}
