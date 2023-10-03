package main

// restriction defines what kind of specific restriction should be performed.
//
// Those are being passed as the first argument to the restrict function, which
// can make some operating system specific decision.
type restriction int

const (
	_ restriction = iota

	// restrict_linux_seccomp: []string as syscallset-go filter
	restrict_linux_seccomp
	// restrict_openbsd_pledge: (string, string) as promises and execpromises for pledge(2)
	restrict_openbsd_pledge
)
