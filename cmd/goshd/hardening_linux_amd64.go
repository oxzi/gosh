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
						"accept",
						"accept4",
						"arch_prctl",
						"bind",
						"clone",
						"close",
						"connect",
						"copy_file_range",
						"dup",
						"epoll_create",
						"epoll_create1",
						"epoll_ctl",
						"epoll_pwait",
						"exit",
						"exit_group",
						"fchdir",
						"fchmod",
						"fchown",
						"fcntl",
						"flock",
						"fstat",
						"fsync",
						"ftruncate",
						"futex",
						"getcwd",
						"getdents64",
						"getpeername",
						"getpid",
						"getrandom",
						"getrlimit",
						"getsockname",
						"getsockopt",
						"gettid",
						"ioctl",
						"kill",
						"listen",
						"lseek",
						"madvise",
						"mincore",
						"mkdirat",
						"mlock",
						"mmap",
						"mprotect",
						"munmap",
						"nanosleep",
						"newfstatat",
						"openat",
						"pipe",
						"pipe2",
						"prctl",
						"pread64",
						"pwrite64",
						"read",
						"readlinkat",
						"recvfrom",
						"recvmsg",
						"renameat",
						"rt_sigaction",
						"rt_sigprocmask",
						"rt_sigreturn",
						"sched_getaffinity",
						"sched_yield",
						"seccomp",
						"sendfile",
						"sendmsg",
						"sendto",
						"set_robust_list",
						"setitimer",
						"setsockopt",
						"shutdown",
						"sigaltstack",
						"socket",
						"splice",
						"tgkill",
						"uname",
						"unlinkat",
						"write",
						"writev",
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
