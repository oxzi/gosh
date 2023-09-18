package internal

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/sys/unix"
)

type StoreRpcMsgType uint

const (
	_ StoreRpcMsgType = iota

	StoreRpcMsgPing

	StoreRpcMsgReqGetItem
	StoreRpcMsgReqGetFile
	StoreRpcMsgReqCreate
	StoreRpcMsgReqDelete
)

const (
	storeRpcBuffSize = 1024
	storeRpcTimeout  = 3 * time.Second
)

type StoreRpcMsg struct {
	Type    StoreRpcMsgType
	Payload interface{} `cbor:",omitempty"`
}

type StoreRpcServer struct {
	Store *Store
	Conn  *net.UnixConn
	Ctx   context.Context
}

type StoreRpcClient struct {
	Conn *net.UnixConn
}

func Socketpair() (parent, child *os.File, err error) {
	fds, err := unix.Socketpair(
		unix.AF_UNIX,
		unix.SOCK_STREAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK,
		0)
	if err != nil {
		return
	}

	parent = os.NewFile(uintptr(fds[0]), "")
	child = os.NewFile(uintptr(fds[1]), "")
	return
}

func UnixConnFromFile(f *os.File) (*net.UnixConn, error) {
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

func (server *StoreRpcServer) Serve() error {
	go func() {
		_ = <-server.Ctx.Done()
		_ = server.Conn.Close()
	}()

	var (
		b   []byte = make([]byte, storeRpcBuffSize)
		msg StoreRpcMsg
	)

	for {
		n, _, _, _, err := server.Conn.ReadMsgUnix(b, nil)
		if err != nil {
			return err
		}

		err = cbor.Unmarshal(b[:n], &msg)
		if err != nil {
			return err
		}

		switch msg.Type {
		case StoreRpcMsgPing:
			err = server.ping()

		default:
			err = fmt.Errorf("received StoreRpcMsg with unsupported type %v", msg.Type)
		}
		if err != nil {
			return err
		}
	}
}

func (server *StoreRpcServer) reply(msg StoreRpcMsg, oob []byte) error {
	b, err := cbor.Marshal(msg)
	if err != nil {
		return err
	}

	_, _, err = server.Conn.WriteMsgUnix(b, oob, nil)
	return err
}

func (client *StoreRpcClient) send(msg StoreRpcMsg, ctx context.Context) error {
	b, err := cbor.Marshal(msg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, storeRpcTimeout)
	defer cancel()

	errChan := make(chan error)
	go func() {
		_, _, err := client.Conn.WriteMsgUnix(b, nil, nil)
		errChan <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case err := <-errChan:
		return err
	}
}

func (client *StoreRpcClient) receive(ctx context.Context) (StoreRpcMsg, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, storeRpcTimeout)
	defer cancel()

	var msg StoreRpcMsg

	feedbackChan := make(chan struct {
		b   []byte
		oob []byte
		err error
	})
	go func() {
		b := make([]byte, storeRpcBuffSize)
		oob := make([]byte, storeRpcBuffSize)
		n, oobn, _, _, err := client.Conn.ReadMsgUnix(b, oob)

		feedbackChan <- struct {
			b   []byte
			oob []byte
			err error
		}{b[:n], oob[:oobn], err}
	}()

	select {
	case <-ctx.Done():
		return msg, nil, ctx.Err()

	case feedback := <-feedbackChan:
		if feedback.err != nil {
			return msg, nil, feedback.err
		}

		err := cbor.Unmarshal(feedback.b, &msg)
		if err != nil {
			return msg, nil, err
		}

		if len(feedback.oob) > 0 {
			return msg, feedback.oob, nil
		}
		return msg, nil, nil
	}
}

func (client *StoreRpcClient) Ping(ctx context.Context) error {
	err := client.send(StoreRpcMsg{
		Type:    StoreRpcMsgPing,
		Payload: nil,
	}, ctx)
	if err != nil {
		return err
	}

	msg, _, err := client.receive(ctx)
	if err != nil {
		return err
	}

	if msg.Type != StoreRpcMsgPing {
		return fmt.Errorf("response has wrong type %v", msg.Type)
	}
	return nil
}

func (server *StoreRpcServer) ping() error {
	return server.reply(StoreRpcMsg{
		Type:    StoreRpcMsgPing,
		Payload: nil,
	}, nil)
}
