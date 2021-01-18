package internal

import (
	"bytes"
	"crypto/rand"
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

	"golang.org/x/crypto/nacl/secretbox"

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

	Filename      string
	FilenameNonce [NonceSize]byte

	ContentType string

	Chunks      uint64
	ChunkSize   uint64
	ChunkNonces [][NonceSize]byte

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
func NewItem(r *http.Request, maxSize int64, maxLifetime time.Duration, chunkSize uint64) (item Item, file io.ReadCloser, err error) {
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

	item.ChunkSize = chunkSize

	return
}

// target returns the path to the Item's file/folder/whatever.
func (i Item) target(directory string) string {
	return filepath.Join(directory, i.ID)
}

// WriteFile serializes the file of an Item in the given directory. The file
// name will be the ID of the Item.
func (i Item) WriteFile(file io.ReadCloser, directory string) error {
	f, err := os.Create(i.target(directory))
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
	return os.Open(i.target(directory))
}

// WriteEncryptedFile splits the file into chunks of size ChunkSize and encrypts each chunk using nacl secretbox.
// Files will be put into a folder named with the Item ID
func (i Item) WriteEncryptedFile(file io.ReadCloser, secretKey [KeySize]byte, directory string) (uint64, [][NonceSize]byte, error) {
	chunkFolder := i.target(directory)
	err := os.Mkdir(chunkFolder, 0700)
	if err != nil {
		return 0, nil, err
	}

	buff := make([]byte, i.ChunkSize)
	chunkNonces := make([][NonceSize]byte, 0)
	var chunkNumber uint64 = 0

	for {
		n, err := file.Read(buff)
		if err == io.EOF {
			log.WithField("chunks", chunkNumber).Debug("Wrote chunked file")
			break
		}
		if err != nil {
			return 0, nil, err
		}

		if n > 0 {
			filename := fmt.Sprintf("%v.chunk", chunkNumber)
			f, err := os.Create(filepath.Join(chunkFolder, filename))
			if err != nil {
				return 0, nil, err
			}

			var nonce [NonceSize]byte
			if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
				return 0, nil, err
			}

			encrypted := secretbox.Seal(nil, buff[:n], &nonce, &secretKey)
			chunkNonces = append(chunkNonces, nonce)

			_, err = f.Write(encrypted)
			if err != nil {
				return 0, nil, err
			}

			chunkNumber += 1
		}
	}

	return chunkNumber, chunkNonces, file.Close()
}

// ReadEncryptedFile takes the chunks written by WriteEncryptedFile, decrypts and verifies each chunk
// and reassembles the original file in memory (the decrypted contents are never written to disk).
func (i Item) ReadEncryptedFile(directory string, secretKey [KeySize]byte) (io.ReadCloser, error) {
	var content bytes.Buffer
	var chunkNumber uint64 = 0
	chunkFolder := i.target(directory)
	// there is some overhead when encrypting, so the read buffer for the chunks must be a bit larger
	buff := make([]byte, i.ChunkSize*2)

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
			nonce := i.ChunkNonces[chunkNumber]

			decrypted, ok := secretbox.Open(nil, buff[:n], &nonce, &secretKey)
			if !ok {
				return nil, ErrDecryptionError
			}

			content.Write(decrypted)
		}
	}

	return ioutil.NopCloser(&content), nil
}

// DeleteContent removes the content of an Item from the given directory.
func (i Item) DeleteContent(directory string) error {
	return os.RemoveAll(i.target(directory))
}
