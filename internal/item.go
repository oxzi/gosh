package internal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	log "github.com/sirupsen/logrus"
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

	// RemoteAddr
	if remoteAddr, _, raErr := net.SplitHostPort(r.RemoteAddr); raErr != nil {
		err = raErr
		return
	} else if remoteAddrIp := net.ParseIP(remoteAddr); remoteAddrIp == nil {
		err = fmt.Errorf("failed to parse the remote IP \"%s\"", remoteAddr)
		return
	} else {
		owners[RemoteAddr] = remoteAddrIp
	}

	// Header
	for _, headerKey := range ownerHeaders {
		if headerVal := r.Header.Get(string(headerKey)); headerVal != "" {
			if headerIp := net.ParseIP(headerVal); headerIp == nil {
				err = fmt.Errorf("failed to parse an IP for %s from \"%s\"", headerKey, headerVal)
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
	Chunks      uint

	Created time.Time
	Expires time.Time `badgerholdIndex:"Expires"`

	Owner map[OwnerType]net.IP
}

var (
	ErrLifetimeToLong = errors.New("lifetime is greater than maximum lifetime")

	ErrFileToBig = errors.New("file size is greater than maximum filesize")

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
		err = ErrFileToBig
		return
	} else if fileHeader.Size == 0 {
		err = fmt.Errorf("file size is zero")
		return
	}

	if burnAfterReading := r.FormValue(formBurnAfterReading); burnAfterReading == "1" {
		item.BurnAfterReading = true
	}

	item.Filename = filenamePattern.ReplaceAllString(
		filepath.Base(filepath.Clean(fileHeader.Filename)), "_")

	if contentType := fileHeader.Header.Get("Content-Type"); contentType == "" {
		err = fmt.Errorf("failed to get a Content-Type from file header")
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
		err = ErrLifetimeToLong
		return
	} else {
		item.Expires = item.Created.Add(parseLt)
	}

	if item.Owner, err = NewOwnerTypes(r); err != nil {
		return
	}

	return
}

// targetDirectory returns the path to the Item's file.
func (i Item) targetDirectory(directory string) string {
	return filepath.Join(directory, i.ID)
}

// WriteFile serializes the file of an Item in the given directory. The file
// name will be the ID of the Item.
func (i Item) WriteFile(file io.ReadCloser, directory string) (uint, error) {
	chunkFolder := i.targetDirectory(directory)
	err := os.Mkdir(chunkFolder, 0700)
	if err != nil {
		return 0, err
	}

	buff := make([]byte, ChunkSize)
	var chunkNumber uint = 0

	for {
		n, err := file.Read(buff)
		if err == io.EOF {
			log.WithFields(log.Fields{
				"chunks": chunkNumber,
			}).Debug("Wrote chunked file")
			break
		}
		if err != nil {
			return 0, err
		}

		if n > 0 {
			filename := fmt.Sprintf("%v.chunk", chunkNumber)
			f, err := os.Create(filepath.Join(chunkFolder, filename))
			if err != nil {
				return 0, err
			}

			_, err = f.Write(buff[:n])
			if err != nil {
				return 0, err
			}
			chunkNumber += 1
		}
	}

	return chunkNumber, file.Close()
}

// ReadFile deserializes the file of an Item from the given directory into a ReadCloser.
func (i Item) ReadFile(directory string) (io.ReadCloser, error) {
	var content bytes.Buffer
	var chunkNumber uint = 0
	chunkFolder := i.targetDirectory(directory)
	buff := make([]byte, ChunkSize)

	for ; chunkNumber < i.Chunks; chunkNumber++ {
		filename := fmt.Sprintf("%v.chunk", chunkNumber)
		file, err := os.Open(filepath.Join(chunkFolder, filename))
		if err != nil {
			return nil, err
		}

		n, err := file.Read(buff)
		if err != nil {
			return nil, err
		}

		if n > 0 {
			content.Write(buff[:n])
		}
	}

	return ioutil.NopCloser(&content), nil
}

// DeleteContent removes the content of an Item from the given directory.
func (i Item) DeleteContent(directory string) error {
	return os.RemoveAll(i.targetDirectory(directory))
}
