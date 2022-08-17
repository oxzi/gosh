//go:build openbsd

package internal

import (
	"strings"

	log "github.com/sirupsen/logrus"

	syscall "golang.org/x/sys/unix"
)

// unveil path to be used by OpenBSD's unveil.
func (opts *HardeningOpts) unveil() {
	if err := syscall.Unveil(*(opts.StoreDir), "rwc"); err != nil {
		log.WithError(err).Fatal("Cannot unveil store directory")
	}

	if opts.ListenUnixAddr != nil {
		if err := syscall.Unveil(*(opts.ListenUnixAddr), "rw"); err != nil {
			log.WithError(err).Fatal("Cannot unveil Unix domain socket")
		}
	}

	if err := syscall.UnveilBlock(); err != nil {
		log.WithError(err).Fatal("Cannot unveil(NULL, NULL)")
	}
}

// pledge to only use some syscall subsets by OpenBSD's pledge.
func (opts *HardeningOpts) pledge() {
	promises := []string{
		"stdio",
		"rpath",
		"wpath",
		"cpath",
		"flock",
		"tty",
		"proc",
	}

	if opts.ListenTcpAddr != nil {
		promises = append(promises, "inet")
	}
	if opts.ListenUnixAddr != nil {
		promises = append(promises, "unix")
	}

	if err := syscall.PledgePromises(strings.Join(promises, " ")); err != nil {
		log.WithError(err).Fatal("Cannot pledge")
	}
}

// Apply both Unix and OpenBSD specific hardening.
func (opts *HardeningOpts) Apply() {
	opts.applyUnix()
	opts.unveil()
	opts.pledge()
}
