package internal

import (
	"context"
	"os"
	"testing"
)

func TestStoreRpcSession(t *testing.T) {
	serverSocket, clientSocket, err := Socketpair()
	if err != nil {
		t.Fatal(err)
	}

	serverUnixSocket, err := UnixConnFromFile(serverSocket)
	if err != nil {
		t.Fatal(err)
	}
	clientUnixSocket, err := UnixConnFromFile(clientSocket)
	if err != nil {
		t.Fatal(err)
	}

	storageDir, err := os.MkdirTemp("", "db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(storageDir)

	store, err := NewStore(storageDir, false)
	if err != nil {
		t.Fatal(err)
	}

	server := &StoreRpcServer{
		Store: store,
		Conn:  serverUnixSocket,
		Ctx:   context.Background(),
	}

	client := &StoreRpcClient{
		Conn: clientUnixSocket,
	}

	serverErrChan := make(chan error, 2)
	go func() {
		serverErrChan <- server.Serve()
	}()

	for i := 0; i < 1024; i++ {
		err = client.Ping(context.Background())
		if err != nil {
			t.Error(err)
		}
	}

	serverErrChan <- nil // populate at least one element; buffered chan
	err = <-serverErrChan
	if err != nil {
		t.Error(err)
	}
}
