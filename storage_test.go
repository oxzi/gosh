package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

// dummyReadCloser wraps around a bytes.Buffer and implements a ReadCloser.
type dummyReadCloser struct {
	buff *bytes.Buffer
}

func newDummyReadCloser(b *bytes.Buffer) dummyReadCloser {
	return dummyReadCloser{buff: b}
}

func (drc dummyReadCloser) Read(p []byte) (int, error) {
	return drc.buff.Read(p)
}

func (drc dummyReadCloser) Close() error {
	return nil
}

func TestStore(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	item := Item{
		ID:      "foobar",
		Expires: time.Now().Add(time.Minute).UTC(),
	}
	itemData := newDummyReadCloser(bytes.NewBuffer([]byte("hello world")))

	storageDir, err := ioutil.TempDir("", "db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(storageDir)

	store, err := NewStore(storageDir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get("whatever"); err != ErrNotFound {
		t.Fatal(err)
	}

	if err := store.Put(item, itemData); err != nil {
		t.Fatal(err)
	}

	if itemX, err := store.Get(item.ID); err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(item, itemX) {
		t.Fatalf("Fetched Item mismatches: got %v and expected %v", itemX, item)
	}

	if err := store.Delete(item); err != nil {
		t.Fatal(err)
	} else if _, err := store.Get(item.ID); err != ErrNotFound {
		t.Fatal(err)
	}

	item.Expires = time.Now().Add(-1 * time.Minute).UTC()
	if err := store.Put(item, itemData); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteExpired(); err != nil {
		t.Fatal(err)
	} else if _, err := store.Get(item.ID); err != ErrNotFound {
		t.Fatal(err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
