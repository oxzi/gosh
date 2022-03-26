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
	"github.com/timshannon/badgerhold/v4"
)

const (
	DirDatabase = "db"
	DirStorage  = "data"
)

// ErrNotFound is returned by the `Store.Get` method if there is no Item for
// the requested ID.
var ErrNotFound = errors.New("No Item found for this ID")

// Store stores an index of all Items as well as the pure files.
type Store struct {
	baseDir string

	bh *badgerhold.Store

	hasCleanup bool
	stopSyn    chan struct{}
	stopAck    chan struct{}
}

// NewStore opens or initializes a Store in the given directory. A background
// task for continuous cleaning can be activated.
func NewStore(baseDir string, backgroundCleanup bool) (s *Store, err error) {
	s = &Store{
		baseDir:    baseDir,
		hasCleanup: backgroundCleanup,
	}

	log.WithField("directory", baseDir).Info("Opening Store")

	for _, dir := range []string{baseDir, s.databaseDir(), s.storageDir()} {
		if _, stat := os.Stat(dir); os.IsNotExist(stat) {
			if err = os.Mkdir(dir, 0700); err != nil {
				log.WithError(err).Error("Cannot create directory")
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
				log.WithError(err).Error("Deletion of expired Items failed")
			}
		}
	}
}

// createID creates a random ID for a new Item.
func (s *Store) createID() (id string, err error) {
	// 4 Bytes of randomness -> 4*8 = 32 Bits of randomness
	// 2^32 = 4 294 967 296 possible combinations
	idBuff := make([]byte, 4)

	for i := 0; i < 32; i++ {
		_, err = rand.Read(idBuff)
		if err != nil {
			return
		}

		id = string(base58.Encode(idBuff))

		if err = s.bh.Get(id, Item{}); err == badgerhold.ErrNotFound {
			err = nil
			return
		} else if err != nil {
			return
		}
	}

	err = errors.New("Failed to calculate an ID")
	return
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

// Get an Item by its ID. The Item's file can be accessed with GetFile.
func (s *Store) Get(id string, delExpired bool) (i Item, err error) {
	log.WithField("ID", id).Debug("Requested Item from Store")

	err = s.bh.Get(id, &i)
	if err == badgerhold.ErrNotFound {
		log.WithField("ID", id).Debug("Requested Item was not found")
		err = ErrNotFound
	} else if err != nil {
		log.WithField("ID", id).WithError(err).Error("Requested Item failed")
	} else if err == nil && delExpired && i.Expires.Before(time.Now()) {
		log.WithFields(log.Fields{
			"ID":      id,
			"expires": i.Expires,
		}).Info("Requested Item is expired, will be deleted")

		if err := s.Delete(i); err != nil {
			log.WithError(err).WithField("ID", id).Error("Deletion of expired Item failed")
		}

		err = ErrNotFound
	}

	return
}

// GetFile creates a ReadCloser to the Item's file.
func (s *Store) GetFile(i Item) (io.ReadCloser, error) {
	return i.ReadFile(s.storageDir())
}

// Put a new Item inside the Store. Both a database entry and a file will be created.
func (s *Store) Put(i Item, file io.ReadCloser) (id string, err error) {
	log.Debug("Requested insertion of Item into the Store")

	id, err = s.createID()
	if err != nil {
		log.WithError(err).Error("Creation of an ID for a new Item failed")
		return
	}

	i.ID = id
	log.WithField("ID", i.ID).Debug("Insert Item with assigned ID")

	err = s.bh.Insert(i.ID, i)
	if err != nil {
		log.WithField("ID", i.ID).WithError(err).Error("Insertion of an Item into database failed")
		return
	}

	err = i.WriteFile(file, s.storageDir())
	if err != nil {
		log.WithField("ID", i.ID).WithError(err).Error("Insertion of an Item into storage failed")
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

// Delte an Item. Both the database entry and the file will be removed.
func (s *Store) Delete(i Item) (err error) {
	log.WithField("ID", i.ID).Debug("Requested deletion of Item")

	err = s.bh.Delete(i.ID, i)
	if err != nil {
		log.WithField("ID", i.ID).WithError(err).Error("Deletion of Item from database failed")
		return
	}

	err = i.DeleteFile(s.storageDir())
	if err != nil {
		log.WithField("ID", i.ID).WithError(err).Error("Deletion of Item from storage failed")
		return
	}

	return
}

// BadgerHold returns a reference to the underlying BadgerHold instance.
func (s *Store) BadgerHold() *badgerhold.Store {
	return s.bh
}
