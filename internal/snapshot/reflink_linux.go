//go:build linux

package snapshot

import (
	"fmt"
	"os"
	"syscall"
)

// ficlone is the Linux FICLONE ioctl (_IOW(0x94, 9, int)): clone a file's data
// extents into the open destination fd with copy-on-write semantics.
const ficloneIoctl = 0x40049409

// reflink clones src into dst via FICLONE on XFS/Btrfs. On filesystems without
// reflink support (ext4, tmpfs), the ioctl fails and the caller falls back to
// a full copy.
func reflink(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer df.Close()
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, df.Fd(), ficloneIoctl, sf.Fd())
	if errno != 0 {
		return fmt.Errorf("FICLONE: %w", syscall.Errno(errno))
	}
	return nil
}
