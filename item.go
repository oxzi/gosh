package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	formFile             string = "file"
	formBurnAfterReading string = "burn"
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

// Item describes an uploaded file.
type Item struct {
	ID string `badgerhold:"key"`

	BurnAfterReading bool

	Filename    string
	ContentType string

	Created time.Time
	Expires time.Time `badgerholdIndex:"Expires"`

	Owner map[OwnerType]net.IP
}

// NewItem creates a new Item based on a Request. Neither the ID nor the
// expiration date (Expires) are set yet. Furthermore, if no error has
// occurred, a file is returned from which the file content should be read.
// This file must be closed afterwards.
func NewItem(r *http.Request, maxSize int64) (item Item, file io.ReadCloser, err error) {
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
			file.Close()
			file = nil
		}
	}()

	if fileHeader.Size > maxSize {
		err = fmt.Errorf("File size %d exceeds maximum %d", fileHeader.Size, maxSize)
		return
	} else if fileHeader.Size == 0 {
		err = fmt.Errorf("File size is zero")
		return
	}

	if burnAfterReading := r.FormValue(formBurnAfterReading); burnAfterReading == "1" {
		item.BurnAfterReading = true
	}

	item.Filename = filepath.Base(filepath.Clean(fileHeader.Filename))

	if contentType := fileHeader.Header.Get("Content-Type"); contentType == "" {
		err = fmt.Errorf("Failed to get a Content-Type from file header")
		return
	} else {
		item.ContentType = contentType
	}

	item.Created = time.Now().UTC()

	if item.Owner, err = NewOwnerTypes(r); err != nil {
		return
	}

	return
}

// WriteFile serializes the file of an Item in the given directory. The file
// name will be the ID of the Item.
func (i Item) WriteFile(file io.ReadCloser, directory string) error {
	target := filepath.Join(directory, i.ID)

	f, err := os.Create(target)
	if err != nil {
		return err
	}

	defer f.Close()

	if _, err := io.Copy(f, file); err != nil {
		return err
	}

	return file.Close()
}

// ReadFile deserializes the file of an Item from the given directory into a Reader.
func (i Item) ReadFile(directory string) (file io.Reader, err error) {
	target := filepath.Join(directory, i.ID)
	file, err = os.Open(target)

	return
}
