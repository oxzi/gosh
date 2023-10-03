//go:build openbsd

package main

import (
	"golang.org/x/sys/unix"
)

// pledge restricts system calls by pledge(2).
func pledge(promises, execpromises string) error {
	return unix.Pledge(promises, execpromises)
}

func restrict(op restriction, args ...interface{}) error {
	if op != restrict_openbsd_pledge {
		return nil
	}

	return pledge(args[0].(string), args[1].(string))
}
