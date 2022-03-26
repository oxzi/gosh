// +build !linux

package internal

import (
	"runtime"

	log "github.com/sirupsen/logrus"
)

// Hardening will active some platform-specific hardening.
func Hardening(_ string) {
	log.Debugf("No hardening available for %s/%s", runtime.GOOS, runtime.GOARCH)
}
