// +build !linux,amd64

package main

import (
	"runtime"

	log "github.com/sirupsen/logrus"
)

// hardening will active some platform-specific hardening.
func hardening() {
	log.Debugf("No hardening available for %s/%s", runtime.GOOS, runtime.GOARCH)
}
