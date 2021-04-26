package main

import (
	seccomp "github.com/elastic/go-seccomp-bpf"
	log "github.com/sirupsen/logrus"
)

// hardening activates a seccomp filter for a pre-recoded profile.
func hardening() {
	if !seccomp.Supported() {
		log.Warn("No seccomp support is available")
		return
	}

	filter := seccomp.Filter{
		NoNewPrivs: true,
		Flag:       seccomp.FilterFlagTSync,
		Policy: seccomp.Policy{
			DefaultAction: seccomp.ActionKillProcess,
			Syscalls: []seccomp.SyscallGroup{
				{
					Action: seccomp.ActionAllow,
					Names: []string{
						"accept4",
						"bind",
						"clone",
						"close",
						"epoll_ctl",
						"epoll_pwait",
						"exit_group",
						"fcntl",
						"flock",
						"fstat",
						"fsync",
						"ftruncate",
						"futex",
						"getdents64",
						"getpid",
						"getrandom",
						"getsockname",
						"gettid",
						"ioctl",
						"listen",
						"lseek",
						"madvise",
						"mmap",
						"mprotect",
						"munmap",
						"nanosleep",
						"newfstatat",
						"openat",
						"read",
						"rt_sigprocmask",
						"rt_sigreturn",
						"sched_yield",
						"set_robust_list",
						"setsockopt",
						"sigaltstack",
						"socket",
						"tgkill",
						"unlinkat",
						"write",
					},
				},
			},
		},
	}

	if err := seccomp.LoadFilter(filter); err != nil {
		log.WithError(err).Fatal("Failed to load seccomp filter")
		return
	}

	log.Info("Applied seccomp filter")
}
