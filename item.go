package main

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// OwnerType describes a possible type of an owner, as an IP address. This can
// be the remote address as well as some header field.
type OwnerType string

const (
	RemoteAddr    OwnerType = "RemoteAddr"
	Forwarded     OwnerType = "Forwarded"
	XForwardedFor OwnerType = "X-Forwarded-For"
)

// ownerHeaders are all kinds of OwnerTypes which are header fields.
var ownerHeaders = []OwnerType{Forwarded, XForwardedFor}

// NewOwnerTypes creates a map of OwnerTypes to IP addresses based on a Request.
func NewOwnerTypes(r *http.Request) (owners map[OwnerType]net.IP, err error) {
	owners = make(map[OwnerType]net.IP)

	// RemoteAddr
	if remoteAddr, _, raErr := net.SplitHostPort(r.RemoteAddr); raErr != nil {
		err = raErr
		return
	} else if remoteAddrIp := net.ParseIP(remoteAddr); remoteAddrIp == nil {
		err = fmt.Errorf("Failed to parse the remote IP \"%s\"", remoteAddr)
		return
	} else {
		owners[RemoteAddr] = remoteAddrIp
	}

	// Header
	for _, headerKey := range ownerHeaders {
		if headerVal := r.Header.Get(string(headerKey)); headerVal != "" {
			if headerIp := net.ParseIP(headerVal); headerIp == nil {
				err = fmt.Errorf("Failed to parse an IP for %s from \"%s\"", headerKey, headerVal)
				return
			} else {
				owners[headerKey] = headerIp
			}
		}
	}

	return
}

type Item struct {
	Filename     string
	OriginalName string

	BurnAfterReading bool
	Expires          time.Time

	Owner map[OwnerType]net.IP
}
