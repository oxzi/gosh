//go:build !(aix || linux || darwin || dragonfly || freebsd || openbsd || netbsd || solaris)

package internal

import (
	"net"
	"runtime"

	log "github.com/sirupsen/logrus"
)

// mkListenSocket might create a TCP socket which should be available.
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
		log.Fatal("Unix domain sockets are not supported on this platform")
	}
}

// Apply platform specific hardening. This method panics if it feels like it.
func (opts *HardeningOpts) Apply() {
	opts.mkListenSocket()

	if opts.ChangeUser != nil {
		log.Fatal("Cannot change the user on this platform")
	}

	log.Warnf("No hardening available for %s/%s", runtime.GOOS, runtime.GOARCH)
}
