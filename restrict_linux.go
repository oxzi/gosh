//go:build linux

package main

import (
	"fmt"
	"strings"

	syscallset "github.com/oxzi/syscallset-go"
)

// seccompBpf restricts system calls by a seccomp(2) BPF in syscallset-go syntax.
func seccompBpf(filter string) error {
	if !syscallset.IsSupported() {
		return fmt.Errorf("seccomp-bpf support is unavailable")
	}

	return syscallset.LimitTo(filter)
}

func restrict(op restriction, args ...interface{}) error {
	if op != restrict_linux_seccomp {
		return nil
	}

	return seccompBpf(strings.Join(args[0].([]string), " "))
}
