package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"time"

	"github.com/akamensky/base58"
)

const (
	formFile             string = "file"
	formBurnAfterReading string = "burn"
	formLifetime         string = "time"
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

	// First, extract the RemoteAddr.
	if remoteAddr, _, raErr := net.SplitHostPort(r.RemoteAddr); raErr != nil {
		err = raErr
		return
	} else if remoteAddrIp := net.ParseIP(remoteAddr); remoteAddrIp == nil {
		err = fmt.Errorf("cannot parse remote IP %q", remoteAddr)
		return
	} else {
		owners[RemoteAddr] = remoteAddrIp
	}

	// Then, check f.e. additional IP address indicating header.
	for _, headerKey := range ownerHeaders {
		headerVal := r.Header.Get(string(headerKey))
		if headerVal == "" {
			continue
		}

		headerIp := net.ParseIP(headerVal)
		if headerIp == nil {
			err = fmt.Errorf("cannot parse remote IP %q from header %q", headerVal, headerKey)
			return
		}
		owners[headerKey] = headerIp
	}

	return
}

// Item describes an uploaded file.
type Item struct {
	ID string `badgerhold:"key"`

	DeletionKey string

	BurnAfterReading bool

	Filename    string
	ContentType string

	Created time.Time
	Expires time.Time `badgerholdIndex:"Expires"`

	Owner map[OwnerType]net.IP
}

var (
	ErrLifetimeTooLong = errors.New("lifetime is greater than maximum lifetime")

	ErrFileTooBig = errors.New("file size is greater than maxium filesize")

	filenamePattern = regexp.MustCompile(`[^0-9A-Za-z-_.]`)
)

// NewItemFromRequest creates a new Item based on a Request.
//
// The ID will be left empty. Furthermore, if no error has occurred, a file
// like io.ReadCloser is returned from which the file's content must be read.
// This file must be closed afterwards.
//
// Note, this Item must be passed to the Store to be safed and get an ID.
func NewItemFromRequest(r *http.Request, maxSize int64, maxLifetime time.Duration) (item Item, file io.ReadCloser, err error) {
	err = r.ParseMultipartForm(maxSize)
	if err != nil {
		return
	}

	file, fileHeader, err := r.FormFile(formFile)
	if err != nil {
		return
	}

	defer func() {
		if err != nil {
			item = Item{}
			_ = file.Close()
			file = nil
		}
	}()

	if fileHeader.Size > maxSize {
		err = ErrFileTooBig
		return
	}
	if fileHeader.Size <= 0 {
		err = errors.New("file size is zero")
		return
	}

	delKeyBuff := make([]byte, 24)
	_, err = rand.Read(delKeyBuff)
	if err != nil {
		return
	}
	item.DeletionKey = string(base58.Encode(delKeyBuff))

	if burnAfterReading := r.FormValue(formBurnAfterReading); burnAfterReading == "1" {
		item.BurnAfterReading = true
	}

	item.Filename = filenamePattern.ReplaceAllString(
		filepath.Base(filepath.Clean(fileHeader.Filename)), "_")

	item.ContentType = fileHeader.Header.Get("Content-Type")
	if item.ContentType == "" {
		err = errors.New("missing Content-Type in file header")
		return
	}

	item.Created = time.Now().UTC()

	if lifetime := r.FormValue(formLifetime); lifetime == "" {
		item.Expires = item.Created.Add(maxLifetime)
	} else if parseLt, parseLtErr := ParseDuration(lifetime); parseLtErr != nil {
		err = parseLtErr
		return
	} else if parseLt > maxLifetime {
		err = ErrLifetimeTooLong
		return
	} else {
		item.Expires = item.Created.Add(parseLt)
	}

	item.Owner, err = NewOwnerTypes(r)
	if err != nil {
		return
	}

	return
}
