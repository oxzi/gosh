//go:build !(linux || openbsd)

package main

// restrict has no implementation for those platforms.
func restrict(op restriction, args ...interface{}) error {
	return nil
}
