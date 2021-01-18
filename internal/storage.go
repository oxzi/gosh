package internal

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/akamensky/base58"
	"github.com/timshannon/badgerhold"
	"golang.org/x/crypto/nacl/secretbox"
)

const (
	DirDatabase = "db"
	DirStorage  = "data"
)

const (
	IDSize    = 4
	KeySize   = 32
	NonceSize = 24
)

// ErrNotFound is returned by the `Store.Get` method if there is no Item for
// the requested ID.
var ErrNotFound = errors.New("no Item found for this ID")

// ErrDecryptionError is returned by `Store.Get` if the decryption failed.
// This may be because the wrong key has been provided,
// or because the data has been tampered with.
var ErrDecryptionError = errors.New("decryption error")

// Store stores an index of all Items as well as the pure files.
type Store struct {
	baseDir string
	encrypt bool

	bh *badgerhold.Store

	hasCleanup bool
	stopSyn    chan struct{}
	stopAck    chan struct{}
}

// NewStore opens or initializes a Store in the given directory. A background
// task for continuous cleaning can be activated.
func NewStore(baseDir string, backgroundCleanup bool, encrypt bool) (s *Store, err error) {
	s = &Store{
		baseDir:    baseDir,
		hasCleanup: backgroundCleanup,
		encrypt:    encrypt,
	}

	log.WithField("directory", baseDir).Info("Opening Store")

	for _, dir := range []string{baseDir, s.databaseDir(), s.storageDir()} {
		if _, stat := os.Stat(dir); os.IsNotExist(stat) {
			if err = os.Mkdir(dir, 0700); err != nil {
				return
			}
		}
	}

	opts := badgerhold.DefaultOptions
	opts.Dir = s.databaseDir()
	opts.ValueDir = opts.Dir
	opts.Logger = log.StandardLogger()

	if s.bh, err = badgerhold.Open(opts); err != nil {
		return
	}

	if s.hasCleanup {
		s.stopSyn = make(chan struct{})
		s.stopAck = make(chan struct{})

		go s.cleanupExired()
	}

	return
}

// databaseDir returns the database subdirectory.
func (s Store) databaseDir() string {
	return filepath.Join(s.baseDir, DirDatabase)
}

// storageDir returns the file storage subdirectory.
func (s Store) storageDir() string {
	return filepath.Join(s.baseDir, DirStorage)
}

// cleanupExired runs in a background goroutine to clean up expired Items.
func (s *Store) cleanupExired() {
	var ticker = time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopSyn:
			close(s.stopAck)
			return

		case <-ticker.C:
			if err := s.DeleteExpired(); err != nil {
				log.WithError(err).Warn("Deletion of expired Items errored")
			}
		}
	}
}

// createID creates a random ID for a new Item.
func (s *Store) createID() (id string, err error) {
	// 4 Bytes of randomnes -> 4*8 = 32 Bits of randomness
	// 2^32 = 4 294 967 296 possible combinations
	idBuff := make([]byte, 4)

	for {
		_, err = rand.Read(idBuff)
		if err != nil {
			return
		}

		id = base58.Encode(idBuff)

		if bhErr := s.bh.Get(id, Item{}); bhErr == badgerhold.ErrNotFound {
			return
		} else if bhErr != nil {
			err = bhErr
			return
		}
	}
}

// Close the Store and its database.
func (s *Store) Close() error {
	log.Info("Closing Store")

	if s.hasCleanup {
		close(s.stopSyn)
		<-s.stopAck
	}

	return s.bh.Close()
}

// Get an Item by its ID. The Item's content can be accessed with GetFile.
func (s *Store) Get(id string, delExpired bool) (i Item, err error) {
	log.WithField("ID", id).Debug("Requested Item from Store")

	err = s.bh.Get(id, &i)
	if err == badgerhold.ErrNotFound {
		log.WithField("ID", id).Debug("Requested Item was not found")
		err = ErrNotFound
	} else if err != nil {
		log.WithField("ID", id).WithError(err).Warn("Requested Item errored")
	} else if delExpired && i.Expires.Before(time.Now()) {
		log.WithFields(log.Fields{
			"ID":      id,
			"expires": i.Expires,
		}).Info("Requested Item is expired, will be deleted")

		if err := s.Delete(i); err != nil {
			log.WithError(err).WithField("ID", id).Warn("Deletion of expired Item errored")
		}

		err = ErrNotFound
	}

	return
}

// Get an Item by its ID and decrypt the filename. The Item's content can be accessed with GetFile.
func (s *Store) GetDecrypted(id string, secretKey [KeySize]byte, delExpired bool) (i Item, err error) {
	i, err = s.Get(id, delExpired)
	if err != nil {
		return
	}

	filenameBytes, err := base58.Decode(i.Filename)
	if err != nil {
		return
	}

	decrypted, ok := secretbox.Open(nil, filenameBytes, &i.FilenameNonce, &secretKey)
	if !ok {
		err = ErrDecryptionError
		return
	}
	i.Filename = string(decrypted)

	return
}

// GetFile creates a ReadCloser to the Item's file.
func (s *Store) GetFile(i Item, secretKey [KeySize]byte) (io.ReadCloser, error) {
	if s.encrypt {
		return i.ReadEncryptedFile(s.storageDir(), secretKey)
	} else {
		return i.ReadFile(s.storageDir())
	}
}

// Put a new Item inside the Store. Both a database entry and a file will be created.
func (s *Store) Put(i Item, file io.ReadCloser) (id string, secretKey [KeySize]byte, err error) {
	log.Debug("Requested insertion of Item into the Store")

	id, err = s.createID()
	if err != nil {
		log.WithError(err).Error("Creation of an ID for a new Item errored")
		return
	}

	if s.encrypt {
		// generate a random key, which will be used to encrypt both the filename and the content
		// the key will be appended to the generated URL and not saved anywhere
		if _, err = io.ReadFull(rand.Reader, secretKey[:]); err != nil {
			log.WithError(err).Warn("Error during key creation")
			return
		}

		// encrypt the filename since that might be sensitive
		var nonce [NonceSize]byte
		if _, err = io.ReadFull(rand.Reader, nonce[:]); err != nil {
			log.WithError(err).Warn("Error during nonce creation")
			return
		}
		filename := base58.Encode(secretbox.Seal(nil, []byte(i.Filename), &nonce, &secretKey))
		i.Filename = filename
		i.FilenameNonce = nonce
	}

	i.ID = id
	log.WithField("ID", i.ID).Debug("Insert Item with assigned ID")

	if s.encrypt {
		var chunks uint64
		var nonces [][NonceSize]byte
		chunks, nonces, err = i.WriteEncryptedFile(file, secretKey, s.storageDir())
		if err != nil {
			log.WithField("ID", i.ID).WithError(err).Warn("Insertion of an Item into storage errored")
			return
		}
		i.Chunks = chunks
		i.ChunkNonces = nonces
	} else {
		err = i.WriteFile(file, s.storageDir())
		if err != nil {
			log.WithField("ID", i.ID).WithError(err).Warn("Insertion of an Item into storage errored")
			return
		}
	}

	err = s.bh.Insert(i.ID, i)
	if err != nil {
		log.WithField("ID", i.ID).WithError(err).Warn("Insertion of an Item into database errored")
		return
	}

	return
}

// DeleteExpired checks the Store for expired Items and deletes them.
func (s *Store) DeleteExpired() error {
	var items []Item
	if err := s.bh.Find(&items, badgerhold.Where("Expires").Lt(time.Now())); err != nil {
		return err
	}

	for _, i := range items {
		log.WithField("ID", i.ID).Debug("Delete expired Item")
		if err := s.Delete(i); err != nil {
			return err
		}
	}

	return nil
}

// Delete an Item. Both the database entry and the file will be removed.
func (s *Store) Delete(i Item) (err error) {
	log.WithField("ID", i.ID).Debug("Requested deletion of Item")

	err = s.bh.Delete(i.ID, i)
	if err != nil {
		log.WithField("ID", i.ID).WithError(err).Warn("Deletion of Item from database errored")
		return
	}

	err = i.DeleteContent(s.storageDir())
	if err != nil {
		log.WithField("ID", i.ID).WithError(err).Warn("Deletion of Item from storage errored")
		return
	}

	return
}

// BadgerHold returns a reference to the underlying BadgerHold instance.
func (s *Store) BadgerHold() *badgerhold.Store {
	return s.bh
}
