//go:build !linux
// +build !linux

package internal

import (
	"runtime"

	log "github.com/sirupsen/logrus"
)

// Hardening will active some platform-specific hardening.
func Hardening(useNetwork bool, storePath string) {
	log.Warnf("No hardening available for %s/%s", runtime.GOOS, runtime.GOARCH)
}
