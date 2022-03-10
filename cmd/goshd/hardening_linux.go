package main

import (
	syscallset "github.com/oxzi/syscallset-go"
	log "github.com/sirupsen/logrus"
)

// hardening activates a seccomp-bpf filter.
func hardening() {
	if !syscallset.IsSupported() {
		log.Warn("No seccomp-bpf support is available")
		return
	}

	filter := "@system-service ~@chown ~@clock ~@cpu-emulation ~@debug ~@keyring ~@memlock ~@module ~@mount ~@privileged ~@reboot ~@resources ~@setuid ~@swap"
	if err := syscallset.LimitTo(filter); err != nil {
		log.WithError(err).Fatal("Failed to apply seccomp-bpf filter")
		return
	}
}
