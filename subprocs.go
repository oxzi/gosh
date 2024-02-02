package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"strconv"

	"golang.org/x/sys/unix"
)

// pipe2 is a helper function wrapper around pipe2 from pipe(2).
//
// Even as pipe2 itself does not seems to be POSIX, it is at least implemented
// by FreeBSD, NetBSD, OpenBSD, and Linux. It seems like the only advantage of
// pipe2 over pipe in this use case is the non-blocking IO.
func pipe2() (reader, writer *os.File, err error) {
	fds := make([]int, 2)
	err = unix.Pipe2(fds, unix.O_NONBLOCK)
	if err != nil {
		return
	}

	reader = os.NewFile(uintptr(fds[0]), "")
	writer = os.NewFile(uintptr(fds[1]), "")
	return
}

// socketpair is a helper function wrapped around socketpair(2).
func socketpair() (parent, child *os.File, err error) {
	fds, err := unix.Socketpair(
		unix.AF_UNIX,
		unix.SOCK_STREAM|unix.SOCK_NONBLOCK,
		0)
	if err != nil {
		return
	}

	parent = os.NewFile(uintptr(fds[0]), "")
	child = os.NewFile(uintptr(fds[1]), "")
	return
}

// forkChild forks off a subprocess for the given child subroutine.
//
// The child process' output will be printed to this process' output. The
// extraFiles are additional file descriptors for communication.
func forkChild(child string, extraFiles []*os.File) (*os.Process, error) {
	logParent, logChild, err := pipe2()
	if err != nil {
		return nil, err
	}

	go func() {
		scanner := bufio.NewScanner(logParent)
		for scanner.Scan() {
			childLogEntry := scanner.Text()
			childLogRecord := make(map[string]any)

			err := json.Unmarshal([]byte(childLogEntry), &childLogRecord)
			if err != nil {
				slog.Warn("Unparsable child message",
					slog.String("child", child), slog.String("msg", childLogEntry),
					slog.Any("error", err))
				continue
			}

			logger := slog.With(slog.String("child", child))
			for k, v := range childLogRecord {
				switch k {
				case "time", "level", "msg":
				default:
					logger = logger.With(slog.Any(k, v))
				}
			}

			levelVal, ok := childLogRecord["level"]
			if !ok {
				slog.Warn("Child messages misses level",
					slog.String("child", child), slog.String("msg", childLogEntry))
				continue
			}

			level := new(slog.Level)
			err = level.UnmarshalText([]byte(levelVal.(string)))
			if err != nil {
				slog.Warn("Failed to parse child's log level",
					slog.String("child", child), slog.String("msg", childLogEntry),
					slog.Any("error", err))
				continue
			}

			logger.Log(context.Background(), *level, childLogRecord["msg"].(string))
		}
		if err := scanner.Err(); err != nil {
			slog.Error("Scanner failed", slog.Any("error", err))
		}
	}()

	cmd := exec.Command(os.Args[0], append(os.Args[1:], "-fork-child", child)...)

	cmd.Env = []string{}
	cmd.Stdin = nil
	cmd.Stdout = logChild
	cmd.Stderr = logChild
	cmd.ExtraFiles = extraFiles

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd.Process, nil
}

// procWait waits for the given Process and eventually closes the channel.
func procWait(ch chan<- struct{}, proc *os.Process) {
	go func() {
		_, _ = proc.Wait()
		close(ch)
	}()
}

// uidGidForUserGroup fetches an UID and GID for the given user and group.
func uidGidForUserGroup(username, groupname string) (uid, gid int, err error) {
	userStruct, err := user.Lookup(username)
	if err != nil {
		return
	}
	userId, err := strconv.ParseInt(userStruct.Uid, 10, 64)
	if err != nil {
		return
	}
	groupStruct, err := user.LookupGroup(groupname)
	if err != nil {
		return
	}
	groupId, err := strconv.ParseInt(groupStruct.Gid, 10, 64)
	if err != nil {
		return
	}

	uid, gid = int(userId), int(groupId)
	return
}

// posixPermDrop uses (more or less) POSIX defined options to drop privileges.
//
// Frist, a chroot is set to the given path. Afterwards, the effective UID and
// GID are being set to those of the given user and group.
//
// It says "more or less POSIX" as setresuid(2) and setresgid(2) aren't part of
// any standard (yet), but are supported by most operating systems.
func posixPermDrop(chroot, username, groupname string) error {
	uid, gid, err := uidGidForUserGroup(username, groupname)
	if err != nil {
		return err
	}

	err = unix.Chroot(chroot)
	if err != nil {
		return fmt.Errorf("chroot: %w", err)
	}
	err = unix.Chdir("/")
	if err != nil {
		return fmt.Errorf("chdir: %w", err)
	}

	err = unix.Setgroups([]int{gid})
	if err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	err = unix.Setresgid(gid, gid, gid)
	if err != nil {
		return fmt.Errorf("setresgid: %w", err)
	}
	err = unix.Setresuid(uid, uid, uid)
	if err != nil {
		return fmt.Errorf("setresuid: %w", err)
	}

	return nil
}
