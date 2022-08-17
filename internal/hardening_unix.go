//go:build aix || darwin || dragonfly || freebsd || netbsd || solaris

package internal

// Apply the generic Unix hardening if there isn't anything more specific.
func (opts *HardeningOpts) Apply() {
	opts.applyUnix()
}
