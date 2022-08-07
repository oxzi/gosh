package internal

import (
	"strings"

	log "github.com/sirupsen/logrus"

	syscallset "github.com/oxzi/syscallset-go"
)

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

// Hardening is achieved on Linux with seccomp-bpf.
func Hardening(useNetwork bool, storePath string) {
	hardeningSeccompBpf(useNetwork)
}
