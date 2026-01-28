package main

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/akamensky/base58"
	"github.com/timshannon/badgerhold/v4"
)

const (
	DirDatabase = "db"
	DirStorage  = "data"
)

// ErrNotFound is returned by the `Store.Get` method if there is no Item for
// the requested ID.
var ErrNotFound = errors.New("no item found for this ID")

// BadgerLogWapper implements badger.Logger to forward logs to log/slog.
type BadgerLogWapper struct {
	*slog.Logger
}

func (logger *BadgerLogWapper) Errorf(f string, args ...interface{}) {
	logger.Error(fmt.Sprintf(f, args...), slog.String("producer", "badger"))
}

func (logger *BadgerLogWapper) Warningf(f string, args ...interface{}) {
	logger.Warn(fmt.Sprintf(f, args...), slog.String("producer", "badger"))
}

func (logger *BadgerLogWapper) Infof(f string, args ...interface{}) {
	logger.Info(fmt.Sprintf(f, args...), slog.String("producer", "badger"))
}

func (logger *BadgerLogWapper) Debugf(f string, args ...interface{}) {
	logger.Debug(fmt.Sprintf(f, args...), slog.String("producer", "badger"))
}

// randomIdGenerator returns an ID generator for the "random" type.
func randomIdGenerator(length int) func() (string, error) {
	return func() (string, error) {
		// n bytes or randomness, which would be for n = 4:
		// 4*8 = 32 Bits of randomness; 2^32 = 4 294 967 296 possible combinations
		idBuff := make([]byte, length)

		_, err := rand.Read(idBuff)
		if err != nil {
			return "", err
		}

		return string(base58.Encode(idBuff)), nil
	}
}

// wordlistIdGenerator returns an ID generator for the "wordlist" type.
func wordlistIdGenerator(sourceFile string, length int) (func() (string, error), error) {
	f, err := os.Open(sourceFile)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)

	words := make([]string, 0, 1024)
	for scanner.Scan() {
		words = append(words, scanner.Text())
	}
	err = scanner.Err()
	if err != nil {
		return nil, err
	}

	return func() (string, error) {
		parts := make([]string, length)
		for i := 0; i < length; i++ {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(words))))
			if err != nil {
				return "", err
			}
			parts[i] = words[int(n.Int64())]
		}

		return strings.Join(parts, "-"), nil
	}, nil
}

// Store stores an index of all Items as well as the pure files.
type Store struct {
	baseDir string

	bh *badgerhold.Store

	idGenerator func() (string, error)

	cleanup bool
	stopSyn chan struct{}
	stopAck chan struct{}
}

// NewStore opens or initializes a Store in the given directory.
//
// autoCleanup specifies if both a background cleanup job will be launched as
// well as deleting expired Items after being retrieved.
func NewStore(
	baseDir string,
	idGenerator func() (string, error),
	autoCleanup bool,
) (s *Store, err error) {
	s = &Store{
		baseDir:     baseDir,
		idGenerator: idGenerator,
		cleanup:     autoCleanup,
	}

	slog.Info("Opening Store", slog.String("directory", baseDir))

	for _, dir := range []string{baseDir, s.databaseDir(), s.storageDir()} {
		_, stat := os.Stat(dir)
		if !os.IsNotExist(stat) {
			continue
		}

		err = os.Mkdir(dir, 0700)
		if err != nil {
			slog.Error("Cannot create directory", slog.String("directory", dir), slog.Any("error", err))
			return
		}
	}

	opts := badgerhold.DefaultOptions
	opts.Dir = s.databaseDir()
	opts.ValueDir = opts.Dir
	opts.Logger = &BadgerLogWapper{slog.Default()}
	opts.BaseLevelSize = 1 << 21    // 2MiB
	opts.ValueLogFileSize = 1 << 24 // 16MiB
	opts.BaseTableSize = 1 << 20    // 1MiB

	s.bh, err = badgerhold.Open(opts)
	if err != nil {
		return
	}

	if s.cleanup {
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
			if err := s.deleteExpired(); err != nil {
				slog.Error("Deletion of expired Items failed", slog.Any("error", err))
			}
		}
	}
}

// createID creates an ID for a new Item based on the Store.idGenerator.
func (s *Store) createID() (string, error) {
	for i := 0; i < 32; i++ {
		id, err := s.idGenerator()
		if err != nil {
			return "", err
		}

		err = s.bh.Get(id, Item{})
		switch err {
		case nil:
			// Continue if this ID is already in use
			continue

		case badgerhold.ErrNotFound:
			// Use this ID if there is no such entry
			return id, nil

		default:
			// Otherwise, pass error along
			return "", err
		}
	}

	return "", errors.New("failed to calculate a free ID")
}

// Close the Store and its database.
func (s *Store) Close() error {
	slog.Info("Closing Store")

	if s.cleanup {
		close(s.stopSyn)
		<-s.stopAck
	}

	return s.bh.Close()
}

// Get an Item by its ID. The Item's file can be accessed with GetFile.
func (s *Store) Get(id string) (i Item, err error) {
	slog.Debug("Requested Item from Store", slog.String("id", id))

	err = s.bh.Get(id, &i)
	if err == badgerhold.ErrNotFound {
		slog.Debug("Requested Item was not found", slog.String("id", id))
		err = ErrNotFound
		return
	} else if err != nil {
		slog.Error("Requesting Item failed", slog.String("id", id))
		return
	}

	if s.cleanup && i.Expires.Before(time.Now()) {
		slog.Info("Requested Item is expired, will be deleted",
			slog.String("id", id), slog.Any("expires", i.Expires))

		err = s.Delete(i.ID)
		if err != nil {
			slog.Error("Failed to delete expired Item", slog.String("id", id), slog.Any("error", err))
			return
		}

		err = ErrNotFound
	}

	return
}

// GetFile creates a ReadCloser for a stored Item file by this ID.
func (s *Store) GetFile(id string) (*os.File, error) {
	return os.Open(filepath.Join(s.storageDir(), id))
}

// Put a new Item inside the Store.
//
// Both a database entry and a file will be created. The given file will be
// read into the storage and closed afterwards.
func (s *Store) Put(i Item, file io.ReadCloser) (id string, err error) {
	slog.Debug("Requested insertion of Item into the Store")

	id, err = s.createID()
	if err != nil {
		slog.Error("Failed to create an ID for a new Item", slog.Any("error", err))
		return
	}

	i.ID = id
	slog.Debug("Insert Item with assigned ID", slog.String("id", i.ID))

	err = s.bh.Insert(i.ID, i)
	if err != nil {
		slog.Error("Failed to insert Item into database",
			slog.String("id", i.ID), slog.Any("error", err))
		return
	}

	f, err := os.Create(filepath.Join(s.storageDir(), i.ID))
	if err != nil {
		slog.Error("Failed to create file",
			slog.String("id", i.ID), slog.Any("error", err))
		return
	}

	_, err = io.Copy(f, file)
	if err != nil {
		return
	}

	err = file.Close()
	if err != nil {
		return
	}

	err = f.Close()
	if err != nil {
		return
	}

	return
}

// deleteExpired checks the Store for expired Items and deletes them.
func (s *Store) deleteExpired() error {
	var items []Item
	err := s.bh.Find(&items, badgerhold.Where("Expires").Lt(time.Now()))
	if err != nil {
		return err
	}

	for _, i := range items {
		slog.Debug("Delete expired Item", slog.String("id", i.ID))
		err := s.Delete(i.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

// Delte an Item. Both the database entry and the file will be removed.
func (s *Store) Delete(id string) (err error) {
	slog.Debug("Requested deletion of Item", slog.String("id", id))

	err = s.bh.Delete(&id, Item{})
	if err != nil {
		slog.Error("Failed to delete Item from database",
			slog.String("id", id), slog.Any("error", err))
		return
	}

	err = os.Remove(filepath.Join(s.storageDir(), id))
	if err != nil {
		slog.Error("Failed to delete Item's file",
			slog.String("id", id), slog.Any("error", err))
		return
	}

	return
}

// BadgerHold returns a reference to the underlying BadgerHold instance.
func (s *Store) BadgerHold() *badgerhold.Store {
	return s.bh
}
