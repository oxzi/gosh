//go:build linux

package internal

import (
	"os"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/landlock-lsm/go-landlock/landlock"
	llsys "github.com/landlock-lsm/go-landlock/landlock/syscall"

	syscallset "github.com/oxzi/syscallset-go"
)

// landlock limits the available paths through Linux' landlock.
func (opts *HardeningOpts) landlock() {
	_, err := llsys.LandlockGetABIVersion()
	if err != nil {
		log.WithError(err).Warn("Landlock is not supported")
		return
	}

	// To restrict a path, it needs to exists as the landlock_add_rule syscall
	// works on an open file descriptor.
	if _, stat := os.Stat(*(opts.StoreDir)); os.IsNotExist(stat) {
		err := os.Mkdir(*(opts.StoreDir), 0700)
		if err != nil {
			log.WithError(err).Fatal("Cannot create store path")
		}
	}

	rwDirs := []string{*(opts.StoreDir)}
	if opts.ListenUnixAddr != nil {
		rwDirs = append(rwDirs, *(opts.ListenUnixAddr))
	}

	if err := landlock.V2.BestEffort().RestrictPaths(landlock.RWDirs(rwDirs...)); err != nil {
		log.WithError(err).Fatal("Failed to apply Landlock filter")
	}
}

// seccompBpf from Linux is used to limit the available syscalls.
func (opts *HardeningOpts) seccompBpf() {
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
	if opts.ListenTcpAddr == nil && opts.ListenUnixAddr == nil {
		filter = append(filter, "~@network-io")
	}

	if err := syscallset.LimitTo(strings.Join(filter, " ")); err != nil {
		log.WithError(err).Fatal("Failed to apply seccomp-bpf filter")
	}
}

// Apply both Unix and Linux specific hardening.
func (opts *HardeningOpts) Apply() {
	opts.applyUnix()
	opts.landlock()
	opts.seccompBpf()
}
