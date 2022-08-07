package internal

import (
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/landlock-lsm/go-landlock/landlock"
	llsys "github.com/landlock-lsm/go-landlock/landlock/syscall"

	syscallset "github.com/oxzi/syscallset-go"
)

// hardeningLandlock with Landlock.
func hardeningLandlock(storePath string) {
	_, err := llsys.LandlockGetABIVersion()
	if err != nil {
		log.Warn("Landlock is not supported")
		return
	}

	storePath, err = filepath.Abs(storePath)
	if err != nil {
		log.WithError(err).Fatal("Cannot create an absolute store path")
	}

	// To restrict a path, it needs to exists as the landlock_add_rule syscall
	// works on an open file descriptor.
	if _, stat := os.Stat(storePath); os.IsNotExist(stat) {
		err := os.Mkdir(storePath, 0700)
		if err != nil {
			log.WithError(err).Fatal("Cannot create store path")
		}
	}

	if err := landlock.V2.BestEffort().RestrictPaths(landlock.RWDirs(storePath)); err != nil {
		log.WithError(err).Fatal("Failed to apply Landlock filter")
	}
}

// hardeningSeccompBpf with a seccomp-bpf filter.
func hardeningSeccompBpf(useNetwork bool) {
	if !syscallset.IsSupported() {
		log.Warn("No seccomp-bpf support is available")
		return
	}

	filter := []string{
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
		"~@resources",
		"~@setuid",
		"~@swap",
		"~execve ~execveat ~fork ~kill",
	}
	if !useNetwork {
		filter = append(filter, "~@network-io")
	}

	if err := syscallset.LimitTo(strings.Join(filter, " ")); err != nil {
		log.WithError(err).Fatal("Failed to apply seccomp-bpf filter")
	}
}

// Hardening is achieved on Linux with Landlock and seccomp-bpf.
func Hardening(useNetwork bool, storePath string) {
	hardeningLandlock(storePath)
	hardeningSeccompBpf(useNetwork)
}
