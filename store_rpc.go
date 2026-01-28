package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// unixConnFromFile converts a file (FD) into an Unix domain socket.
func unixConnFromFile(f *os.File) (*net.UnixConn, error) {
	fConn, err := net.FileConn(f)
	if err != nil {
		return nil, err
	}

	conn, ok := fConn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("cannot use (%T, %T) as *net.UnixConn", f, conn)
	}
	return conn, nil
}

// sendFd sends an open File (resp. its FD) over an Unix domain socket.
func sendFd(f *os.File, conn *net.UnixConn) error {
	oob := unix.UnixRights(int(f.Fd()))
	_, _, err := conn.WriteMsgUnix(nil, oob, nil)
	return err
}

// recvFd receives a File (resp. its FD) from an Unix domain socket.
func recvFd(conn *net.UnixConn) (*os.File, error) {
	oob := make([]byte, 128)
	_, oobn, _, _, err := conn.ReadMsgUnix(nil, oob)
	if err != nil {
		return nil, err
	}

	cmsgs, err := unix.ParseSocketControlMessage(oob[0:oobn])
	if err != nil {
		return nil, err
	} else if len(cmsgs) != 1 {
		return nil, fmt.Errorf("ParseSocketControlMessage: wrong length %d", len(cmsgs))
	}

	fds, err := unix.ParseUnixRights(&cmsgs[0])
	if err != nil {
		return nil, err
	} else if len(fds) != 1 {
		return nil, fmt.Errorf("ParseUnixRights: wrong length %d", len(fds))
	}

	return os.NewFile(uintptr(fds[0]), ""), nil
}

// StoreRpcServer serves a Store over a net/rpc with two connections, one for
// the actual RPC calls (HTTP) and one to pass file descriptors (FDs).
//
// The *StoreRpcServer type implements multiple net/rpc methods. As by creating
// a NewStoreRpcServer it registers itself as an rpc backend, those methods are
// then available to be used by the StoreRpcClient.
type StoreRpcServer struct {
	rpcConn *net.UnixConn
	fdConn  *net.UnixConn

	store     *Store
	rpcServer *rpc.Server
}

// NewStoreRpcServer creates a StoreRpcServer which directly starts listening
// until Close is called.
func NewStoreRpcServer(store *Store, rpcConn, fdConn *net.UnixConn) *StoreRpcServer {
	server := &StoreRpcServer{
		rpcConn: rpcConn,
		fdConn:  fdConn,

		store:     store,
		rpcServer: rpc.NewServer(),
	}

	_ = server.rpcServer.Register(server)
	go server.rpcServer.ServeConn(rpcConn)

	return server
}

// Close this StoreRpcServer and all its connections.
func (server *StoreRpcServer) Close() error {
	_ = server.rpcConn.Close()
	_ = server.fdConn.Close()

	return server.store.Close()
}

// StoreRpcClient is the client to access the Store over this API.
//
// Each client request will be passed with a context.Context as it might be
// initiated from a web server request.
type StoreRpcClient struct {
	rpcClient *rpc.Client
	fdConn    *net.UnixConn
}

// NewStoreRpcClient creates a StoreRpcClient.
func NewStoreRpcClient(rpcConn, fdConn *net.UnixConn) *StoreRpcClient {
	return &StoreRpcClient{
		rpcClient: rpc.NewClient(rpcConn),
		fdConn:    fdConn,
	}
}

// call the net/rpc function with a timeout context.
func (client *StoreRpcClient) call(method string, args interface{}, reply interface{}, ctx context.Context) error {
	timeout, timeoutCancel := context.WithTimeout(ctx, 3*time.Second)
	defer timeoutCancel()

	call := client.rpcClient.Go("StoreRpcServer."+method, args, reply, nil)

	select {
	case <-timeout.Done():
		return ctx.Err()

	case reply := <-call.Done:
		return reply.Error
	}
}

// Close this StoreRpcClient and all its connections.
func (client *StoreRpcClient) Close() error {
	_ = client.rpcClient.Close()
	_ = client.fdConn.Close()

	return nil
}

// Get wraps Store.Get and returns an Item for the requested ID.
func (server *StoreRpcServer) Get(id string, item *Item) error {
	i, err := server.store.Get(id)
	if err != nil {
		return err
	}
	*item = i
	return nil
}

// Get an Item by its ID from the server.
func (client *StoreRpcClient) Get(id string, ctx context.Context) (Item, error) {
	var item Item
	err := client.call("Get", id, &item, ctx)

	// The original error type gets lost..
	if err != nil && err.Error() == ErrNotFound.Error() {
		err = ErrNotFound
	}

	return item, err
}

// GetFile wraps Store.GetFile and sends a FD for the file back.
func (server *StoreRpcServer) GetFile(id string, _ *int) error {
	f, err := server.store.GetFile(id)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	err = sendFd(f, server.fdConn)
	if err != nil {
		return err
	}

	return nil
}

// GetFile returns an *os.File for the requested ID from the server.
func (client *StoreRpcClient) GetFile(id string, ctx context.Context) (*os.File, error) {
	err := client.call("GetFile", id, nil, ctx)
	if err != nil {
		return nil, err
	}

	return recvFd(client.fdConn)
}

// Put wraps Store.Put but reads the input data from a pipe2(2).
//
// Honestly speaking, the pipe2 part is one of my most favourite hacks as the
// StoreRpcClient creates a new pipe - which are just two FDs - and passes the
// reading end over the Unix domain socket to the server to be read into the DB.
func (server *StoreRpcServer) Put(item Item, id *string) error {
	fd, err := recvFd(server.fdConn)
	if err != nil {
		return err
	}

	itemId, err := server.store.Put(item, fd)
	if err != nil {
		return err
	}
	*id = itemId

	return nil
}

// Put a new Item and its data into the server's storage and return the new ID.
func (client *StoreRpcClient) Put(item Item, file io.ReadCloser, ctx context.Context) (string, error) {
	var (
		wg     sync.WaitGroup
		itemId string
		errs   []interface{}
	)

	dataReader, dataWriter, err := pipe2()
	if err != nil {
		return "", err
	}

	const producers = 3
	errChan := make(chan error, producers)
	finChan := make(chan struct{})
	wg.Add(producers)

	go func() {
		_, err := io.Copy(dataWriter, file)
		err2 := dataWriter.Close()
		if err != nil || err2 != nil {
			errChan <- fmt.Errorf("%v %v", err, err2)
		}
		errChan <- nil
		wg.Done()
	}()

	go func() {
		errChan <- sendFd(dataReader, client.fdConn)
		wg.Done()
	}()

	go func() {
		errChan <- client.call("Put", item, &itemId, ctx)
		wg.Done()
	}()

	// This is not a producer, but closes the finChan to allow quick exiting by
	// selecting this channel as well as the given context. Having context being
	// available in sync would be nice..
	go func() {
		wg.Wait()
		close(finChan)
	}()

	timeout, timeoutCancel := context.WithTimeout(ctx, 3*time.Second)
	defer timeoutCancel()

	select {
	case <-finChan:
		break

	case <-timeout.Done():
		errs = append(errs, timeout.Err())
	}

	for i := 0; i < producers; i++ {
		err := <-errChan
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return "", fmt.Errorf(strings.Repeat("%v ", len(errs)), errs...)
	}

	return itemId, nil
}

// Delete wraps Store.Delete.
func (server *StoreRpcServer) Delete(id string, _ *int) error {
	return server.store.Delete(id)
}

// Delete both an Item as well as its file from the server.
func (client *StoreRpcClient) Delete(id string, ctx context.Context) error {
	return client.call("Delete", id, nil, ctx)
}
