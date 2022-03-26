package internal

import (
	syscallset "github.com/oxzi/syscallset-go"
	log "github.com/sirupsen/logrus"
)

// Hardening activates a seccomp-bpf filter.
//
// The default filter can be extended through the parameter.
func Hardening(extraFilter string) {
	if !syscallset.IsSupported() {
		log.Warn("No seccomp-bpf support is available")
		return
	}

	filter := "@system-service ~@chown ~@clock ~@cpu-emulation ~@debug ~@keyring ~@memlock ~@module ~@mount ~@privileged ~@reboot ~@resources ~@setuid ~@swap " + extraFilter
	if err := syscallset.LimitTo(filter); err != nil {
		log.WithError(err).Fatal("Failed to apply seccomp-bpf filter")
		return
	}
}
