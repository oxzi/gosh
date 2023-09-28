package main

import (
	"bufio"
	"context"
	"flag"
	"os"
	"os/exec"
	"os/signal"

	"golang.org/x/sys/unix"

	log "github.com/sirupsen/logrus"
)

// forkChild forks off a subprocess for the given child subroutine.
//
// The child process' output will be printed to this process' output. The
// extraFiles are additional file descriptors for communication.
func forkChild(child string, extraFiles []*os.File, ctx context.Context) (*exec.Cmd, error) {
	logParent, logChild, err := Socketpair()
	if err != nil {
		return nil, err
	}

	go func() {
		scanner := bufio.NewScanner(logParent)
		for scanner.Scan() {
			log.WithField("subprocess", child).Print(scanner.Text())
			if err := scanner.Err(); err != nil {
				log.WithField("subprocess", child).WithError(err).Error("Scanner failed")
			}
		}
	}()

	cmd := exec.CommandContext(ctx, os.Args[0], append(os.Args[1:], "-fork-child", child)...)

	cmd.Env = []string{}
	cmd.Stdin = nil
	cmd.Stdout = logChild
	cmd.Stderr = logChild
	cmd.ExtraFiles = extraFiles

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func main() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	var (
		flagStorePath string
		flagForkChild string
		flagVerbose   bool
	)

	log.WithField("args", os.Args).Info("args")

	flag.StringVar(&flagStorePath, "store", "", "Path to the store")
	flag.StringVar(&flagForkChild, "fork-child", "", "Start a subprocess child")
	flag.BoolVar(&flagVerbose, "verbose", false, "Verbose logging")

	flag.Parse()

	if flagVerbose {
		log.SetLevel(log.DebugLevel)
	}

	switch flagForkChild {
	case "webserver":
		mainWebserver()
		return

	case "store":
		mainStore(flagStorePath)
		return
	}

	storeRpcServer, storeRpcClient, err := Socketpair()
	if err != nil {
		log.Fatal(err)
	}
	storeFdServer, storeFdClient, err := Socketpair()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), unix.SIGINT)
	defer cancel()

	_, err = forkChild("store", []*os.File{storeRpcServer, storeFdServer}, ctx)
	if err != nil {
		log.Fatal(err)
	}

	_, err = forkChild("webserver", []*os.File{storeRpcClient, storeFdClient}, ctx)
	if err != nil {
		log.Fatal(err)
	}

	select {
	case <-ctx.Done():
		log.WithError(ctx.Err()).Error("Context was canceled")
	}
}
