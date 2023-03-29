package internal

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/akamensky/base58"
)

const (
	formFile             string = "file"
	formBurnAfterReading string = "burn"
	formLifetime         string = "period"
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
		err = fmt.Errorf("Failed to parse the remote IP %q", remoteAddr)
		return
	} else {
		owners[RemoteAddr] = remoteAddrIp
	}

	// Header
	for _, headerKey := range ownerHeaders {
		if headerVal := r.Header.Get(string(headerKey)); headerVal != "" {
			if headerIp := net.ParseIP(headerVal); headerIp == nil {
				err = fmt.Errorf("Failed to parse an IP for %q from %q", headerKey, headerVal)
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

	DeletionKey string

	BurnAfterReading bool

	Filename    string
	ContentType string

	Created time.Time
	Expires time.Time `badgerholdIndex:"Expires"`

	Owner map[OwnerType]net.IP
}

var (
	ErrLifetimeTooLong = errors.New("Lifetime is greater than maximum lifetime")

	ErrFileTooBig = errors.New("File size is greater than maxium filesize")

	filenamePattern = regexp.MustCompile(`[^0-9A-Za-z-_.]`)
)

// NewItem creates a new Item based on a Request. The ID will be left empty.
// Furthermore, if no error has occurred, a file is returned from which the
// file content should be read. This file must be closed afterwards.
func NewItem(r *http.Request, maxSize int64, maxLifetime time.Duration) (item Item, file io.ReadCloser, err error) {
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
		err = ErrFileTooBig
		return
	} else if fileHeader.Size <= 0 {
		err = fmt.Errorf("File size is zero")
		return
	}

	delKeyBuff := make([]byte, 24)
	if _, err = rand.Read(delKeyBuff); err != nil {
		return
	} else {
		item.DeletionKey = string(base58.Encode(delKeyBuff))
	}

	if burnAfterReading := r.FormValue(formBurnAfterReading); burnAfterReading == "1" {
		item.BurnAfterReading = true
	}

	item.Filename = filenamePattern.ReplaceAllString(
		filepath.Base(filepath.Clean(fileHeader.Filename)), "_")

	if contentType := fileHeader.Header.Get("Content-Type"); contentType == "" {
		err = fmt.Errorf("Failed to get a Content-Type from file header")
		return
	} else {
		item.ContentType = contentType
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

	if item.Owner, err = NewOwnerTypes(r); err != nil {
		return
	}

	return
}

// targetFile returns the path to the Item's file.
func (i Item) targetFile(directory string) string {
	return filepath.Join(directory, i.ID)
}

// WriteFile serializes the file of an Item in the given directory. The file
// name will be the ID of the Item.
func (i Item) WriteFile(file io.ReadCloser, directory string) error {
	f, err := os.Create(i.targetFile(directory))
	if err != nil {
		return err
	}

	defer f.Close()

	if _, err := io.Copy(f, file); err != nil {
		return err
	}

	return file.Close()
}

// ReadFile deserializes the file of an Item from the given directory into a ReadCloser.
func (i Item) ReadFile(directory string) (io.ReadCloser, error) {
	return os.Open(i.targetFile(directory))
}

// DeleteFile removes the file of an Item from the given directory.
func (i Item) DeleteFile(directory string) error {
	return os.Remove(i.targetFile(directory))
}
