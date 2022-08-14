//go:build !linux && !openbsd
// +build !linux,!openbsd

package internal

import (
	"os"
	"runtime"

	log "github.com/sirupsen/logrus"
)

// Hardening will active some platform-specific hardening.
//
// socketFd might be a file descriptor to be used for the unix socket.
func Hardening(useNetwork bool, storePath, listenAddr *string, socketFd **os.File) {
	log.Warnf("No hardening available for %s/%s", runtime.GOOS, runtime.GOARCH)
}
