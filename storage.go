package main

import (
	"errors"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/timshannon/badgerhold"
)

var ErrNotFound = errors.New("No Item found for this ID")

type Store struct {
	dbDir   string
	fileDir string

	bh *badgerhold.Store

	stopSyn chan struct{}
	stopAck chan struct{}
}

func NewStore(dbDir, fileDir string) (s *Store, err error) {
	s = &Store{
		dbDir:   dbDir,
		fileDir: fileDir,

		stopSyn: make(chan struct{}),
		stopAck: make(chan struct{}),
	}

	for _, dir := range []string{dbDir, fileDir} {
		if _, stat := os.Stat(dir); os.IsNotExist(stat) {
			if err = os.Mkdir(dir, 0700); err != nil {
				return
			}
		}
	}

	opts := badgerhold.DefaultOptions
	opts.Dir = dbDir
	opts.ValueDir = dbDir

	if s.bh, err = badgerhold.Open(opts); err != nil {
		return
	}

	go s.cleanupExired()

	return
}

func (s *Store) cleanupExired() {
	var ticker = time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopSyn:
			close(s.stopAck)
			return

		case <-ticker.C:
			s.DeleteExpired()
		}
	}
}

func (s *Store) Close() error {
	close(s.stopSyn)
	<-s.stopAck

	return s.bh.Close()
}

func (s *Store) Get(id string) (i Item, err error) {
	err = s.bh.Get(id, &i)
	if err == badgerhold.ErrNotFound {
		err = ErrNotFound
	} else if err == nil && i.Expires.Before(time.Now()) {
		// TODO: handle deletion error
		s.Delete(i)
		err = ErrNotFound
	}
	// TODO: burn after reading

	return
}

func (s *Store) Put(i Item) error {
	log.Info(s)

	// TODO create file
	return s.bh.Insert(i.ID, i)
}

func (s *Store) DeleteExpired() error {
	var items []Item
	if err := s.bh.Find(&items, badgerhold.Where("Expires").Lt(time.Now())); err != nil {
		return err
	}

	for _, i := range items {
		if err := s.Delete(i); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) Delete(i Item) error {
	// TODO delete file
	return s.bh.Delete(i.ID, i)
}
