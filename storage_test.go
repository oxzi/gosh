package main

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	item := Item{
		ID:      "foobar",
		Expires: time.Now().Add(time.Minute).UTC(),
	}

	storeDb, err := ioutil.TempDir("", "db")
	if err != nil {
		t.Fatal(err)
	}
	storeFiles, err := ioutil.TempDir("", "files")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(storeDb)
	defer os.RemoveAll(storeFiles)

	store, err := NewStore(storeDb, storeFiles)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get("whatever"); err != ErrNotFound {
		t.Fatal(err)
	}

	if err := store.Put(item); err != nil {
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
	if err := store.Put(item); err != nil {
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
