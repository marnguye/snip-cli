//go:build unix

package tomlrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

const lockTimeout = 10 * time.Second

func acquireLock(ctx context.Context, path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil && info.IsDir() {
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove legacy snippet lock directory: %w", err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open snippet lock: %w", err)
	}
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			file.Close()
			return nil, fmt.Errorf("acquire snippet lock: %w", err)
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, fmt.Errorf("acquire snippet lock: %w", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
