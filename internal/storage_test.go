package internal

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

	item := Item{Expires: time.Now().Add(time.Minute).UTC()}
	itemData := newDummyReadCloser(bytes.NewBuffer([]byte("hello world")))

	storageDir, err := ioutil.TempDir("", "db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(storageDir)

	store, err := NewStore(storageDir, false, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get("whatever", true); err != ErrNotFound {
		t.Fatal(err)
	}

	itemId, _, err := store.Put(item, itemData)
	if err != nil {
		t.Fatal(err)
	}
	item.ID = itemId

	if itemX, err := store.Get(itemId, true); err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(item, itemX) {
		t.Fatalf("Fetched Item mismatches: got %v and expected %v", itemX, item)
	}

	if err := store.Delete(item); err != nil {
		t.Fatal(err)
	} else if _, err := store.Get(item.ID, true); err != ErrNotFound {
		t.Fatal(err)
	}

	item.Expires = time.Now().Add(-1 * time.Minute).UTC()
	if _, _, err := store.Put(item, itemData); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteExpired(); err != nil {
		t.Fatal(err)
	} else if _, err := store.Get(item.ID, true); err != ErrNotFound {
		t.Fatal(err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreCreateId(t *testing.T) {
	const ids = 1024

	storageDir, err := ioutil.TempDir("", "db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(storageDir)

	store, err := NewStore(storageDir, false, false)
	if err != nil {
		t.Fatal(err)
	}

	idCheck := make(map[string]struct{})
	for i := 0; i < ids; i++ {
		id, err := store.createID()
		if err != nil {
			t.Fatal(err)
		}

		if _, exists := idCheck[id]; exists {
			t.Fatalf("ID %s does already exist", id)
		} else {
			idCheck[id] = struct{}{}
		}
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
