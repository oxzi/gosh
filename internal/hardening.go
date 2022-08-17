package internal

import (
	"os"
)

// HardeningOpts are being altered by platform specific functions to allow
// establishing a state of least privilege.
//
// The topper variables are treated as inputs and might get altered, e.g., after
// entering a chroot. The bottom variables are output variables, being populated
// by platform specific code.
//
// Use the Apply method on a *HardeningOpts.
type HardeningOpts struct {
	// StoreDir is the path to the store; MUST be set.
	StoreDir *string
	// ListenTcpAddr is a listen address for a TCP socket; MIGHT be set.
	ListenTcpAddr *string
	// ListenUnixAddr is the path for a Unix domain socket; MIGHT be set.
	ListenUnixAddr *string
	// ChangeUser is a system user which identity should be used; MIGHT be set.
	ChangeUser *string

	// ListenSocket is a file descriptor to a socket if either ListenTcpAddr or
	// ListenUnixAddr is set.
	ListenSocket *os.File
}
