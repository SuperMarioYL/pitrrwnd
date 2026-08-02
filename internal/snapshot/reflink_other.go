//go:build !linux

package snapshot

// reflink is unsupported on non-Linux platforms (macOS clonefile is not in the
// stdlib syscall surface and adding x/sys is out of scope for v0.1). The
// caller falls back to a full byte-for-byte copy, which is always safe.
func reflink(src, dst string) error {
	return errUnsupported
}
