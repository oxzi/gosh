package internal

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"reflect"
	"testing"
	"time"
)

// testStoreRpcSessionGet sets up a valid Item first, then tests Get.
//
// The logic is borrowed from store_test.go's TestStore.
func testStoreRpcSessionGet(t *testing.T, server *StoreRpcServer, client *StoreRpcClient) {
	item := Item{Expires: time.Now().Add(time.Minute).UTC()}
	itemDataRaw := []byte("hello world")
	itemData := newDummyReadCloser(bytes.NewBuffer(itemDataRaw))

	itemId, err := server.store.Put(item, itemData)
	if err != nil {
		t.Error(err)
	}
	item.ID = itemId

	itemX, err := client.Get(itemId, context.Background())
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(item, itemX) {
		t.Errorf("Fetched Item mismatches: got %v and expected %v", itemX, item)
	}
}

// testStoreRpcSessionGetFile sets up a valid Item first, then tests GetFile.
//
// It builds on top of testStoreRpcSessionGet - duplicate code ahoy!
func testStoreRpcSessionGetFile(t *testing.T, server *StoreRpcServer, client *StoreRpcClient) {
	item := Item{Expires: time.Now().Add(time.Minute).UTC()}
	itemDataRaw := []byte("hello world")
	itemData := newDummyReadCloser(bytes.NewBuffer(itemDataRaw))

	itemId, err := server.store.Put(item, itemData)
	if err != nil {
		t.Error(err)
	}
	item.ID = itemId

	itemX, err := client.Get(itemId, context.Background())
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(item, itemX) {
		t.Errorf("Fetched Item mismatches: got %v and expected %v", itemX, item)
	}

	if f, err := client.GetFile(itemId, context.Background()); err != nil {
		t.Error(err)
	} else {
		buff := make([]byte, len(itemDataRaw))
		n, err := io.ReadFull(f, buff)
		if err != nil {
			t.Error(n, err)
		}
		f.Close()
		buff = buff[:n]

		if !bytes.Equal(itemDataRaw, buff) {
			t.Errorf("Store data mismatch: %v != %v", itemDataRaw, buff)
		}
	}
}

// testStoreRpcSessionPut tests Put'ing a new Item of the given size to the Store.
//
// It builds on top of testStoreRpcSessionGetFile - duplicate code ahoy!
func testStoreRpcSessionPut(size int) func(*testing.T, *StoreRpcServer, *StoreRpcClient) {
	return func(t *testing.T, _ *StoreRpcServer, client *StoreRpcClient) {
		itemDataRaw := make([]byte, size)
		_, err := rand.Read(itemDataRaw)
		if err != nil {
			t.Error(err)
			return
		}

		item := Item{Expires: time.Now().Add(time.Minute).UTC()}
		itemData := newDummyReadCloser(bytes.NewBuffer(itemDataRaw))

		itemId, err := client.Put(item, itemData, context.Background())
		if err != nil {
			t.Error(err)
		}
		item.ID = itemId

		itemX, err := client.Get(itemId, context.Background())
		if err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(item, itemX) {
			t.Errorf("Fetched Item mismatches: got %v and expected %v", itemX, item)
		}

		if f, err := client.GetFile(itemId, context.Background()); err != nil {
			t.Error(err)
		} else {
			buff := make([]byte, len(itemDataRaw))
			n, err := io.ReadFull(f, buff)
			if err != nil {
				t.Error(n, err)
			}
			f.Close()
			buff = buff[:n]

			if !bytes.Equal(itemDataRaw, buff) {
				t.Errorf("Store data mismatch: %v != %v", itemDataRaw, buff)
			}
		}
	}
}

// testStoreRpcSessionDelete tests Delete'ing an Item.
//
// It builds on top of testStoreRpcSessionGetFile - duplicate code ahoy!
func testStoreRpcSessionDelete(t *testing.T, server *StoreRpcServer, client *StoreRpcClient) {
	item := Item{Expires: time.Now().Add(time.Minute).UTC()}
	itemDataRaw := []byte("hello world")
	itemData := newDummyReadCloser(bytes.NewBuffer(itemDataRaw))

	itemId, err := server.store.Put(item, itemData)
	if err != nil {
		t.Error(err)
	}
	item.ID = itemId

	itemX, err := client.Get(itemId, context.Background())
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(item, itemX) {
		t.Errorf("Fetched Item mismatches: got %v and expected %v", itemX, item)
	}

	err = client.Delete(item.ID, context.Background())
	if err != nil {
		t.Error(err)
	}
	_, err = client.Get(itemId, context.Background())
	if err != ErrNotFound {
		t.Error(err)
	}
}

// testStoreRpcSessionSession mimics store_test.go's TestStore.
func testStoreRpcSessionSession(t *testing.T, server *StoreRpcServer, client *StoreRpcClient) {
	item := Item{Expires: time.Now().Add(time.Minute).UTC()}
	itemDataRaw := []byte("hello world")
	itemData := newDummyReadCloser(bytes.NewBuffer(itemDataRaw))

	if _, err := client.Get("whatever", context.Background()); err != ErrNotFound {
		t.Error(err)
	}

	itemId, err := client.Put(item, itemData, context.Background())
	if err != nil {
		t.Error(err)
	}
	item.ID = itemId

	if itemX, err := client.Get(itemId, context.Background()); err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(item, itemX) {
		t.Errorf("Fetched Item mismatches: got %v and expected %v", itemX, item)
	}

	if f, err := client.GetFile(itemId, context.Background()); err != nil {
		t.Error(err)
	} else {
		buff := make([]byte, len(itemDataRaw))
		n, err := io.ReadFull(f, buff)
		if err != nil {
			t.Error(n, err)
		}
		f.Close()
		buff = buff[:n]

		if !bytes.Equal(itemDataRaw, buff) {
			t.Errorf("Store data mismatch: %v != %v", itemDataRaw, buff)
		}
	}

	if err := client.Delete(item.ID, context.Background()); err != nil {
		t.Error(err)
	} else if _, err := client.Get(item.ID, context.Background()); err != ErrNotFound {
		t.Error(err)
	}

	item.Expires = time.Now().Add(-1 * time.Minute).UTC()
	if _, err := client.Put(item, itemData, context.Background()); err != nil {
		t.Error(err)
	}

	if err := server.store.deleteExpired(); err != nil {
		t.Error(err)
	} else if _, err := client.Get(item.ID, context.Background()); err != ErrNotFound {
		t.Error(err)
	}
}

func TestStoreRpcSession(t *testing.T) {
	tests := []struct {
		name string
		f    func(*testing.T, *StoreRpcServer, *StoreRpcClient)
	}{
		{"Get", testStoreRpcSessionGet},
		{"GetFile", testStoreRpcSessionGetFile},
		{"Put-0", testStoreRpcSessionPut(0)},
		{"Put-128", testStoreRpcSessionPut(128)},
		{"Put-1k", testStoreRpcSessionPut(1024)},
		{"Put-1m", testStoreRpcSessionPut(1024 * 1024)},
		{"Put-100m", testStoreRpcSessionPut(100 * 1024 * 1024)},
		{"Delete", testStoreRpcSessionDelete},
		{"Session", testStoreRpcSessionSession},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverRpcSocket, clientRpcSocket, err := Socketpair()
			if err != nil {
				t.Fatal(err)
			}
			serverFdSocket, clientFdSocket, err := Socketpair()
			if err != nil {
				t.Fatal(err)
			}

			serverRpcUnixSocket, err := UnixConnFromFile(serverRpcSocket)
			if err != nil {
				t.Fatal(err)
			}
			clientRpcUnixSocket, err := UnixConnFromFile(clientRpcSocket)
			if err != nil {
				t.Fatal(err)
			}
			serverFdUnixSocket, err := UnixConnFromFile(serverFdSocket)
			if err != nil {
				t.Fatal(err)
			}
			clientFdUnixSocket, err := UnixConnFromFile(clientFdSocket)
			if err != nil {
				t.Fatal(err)
			}

			storageDir, err := os.MkdirTemp("", "db")
			if err != nil {
				t.Fatal(err)
			}

			store, err := NewStore(storageDir, false)
			if err != nil {
				t.Fatal(err)
			}

			server := NewStoreRpcServer(store, serverRpcUnixSocket, serverFdUnixSocket)
			client := NewStoreRpcClient(clientRpcUnixSocket, clientFdUnixSocket)

			test.f(t, server, client)

			err = client.Close()
			if err != nil {
				t.Error(err)
			}
			err = server.Close()
			if err != nil {
				t.Error(err)
			}

			_ = os.RemoveAll(storageDir)
		})
	}
}
