package relay

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

// reservePort is cooperative across relay processes using the same account home.
// Keep the inode after release: unlinking would let contenders lock different files.
func reservePort(port int) (*os.File, error) {
	account, err := user.Current()
	if err == nil {
		// Current may fall back to HOME for unknown UIDs in pure-Go Unix builds.
		// Require the system account record instead of an environment-selected path.
		account, err = user.LookupId(account.Uid)
	}
	if err != nil || account.HomeDir == "" {
		return nil, errors.New("cannot locate the account home for relay port reservation")
	}
	dir := filepath.Join(account.HomeDir, ".zelport", "relay-locks")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create relay reservation directory: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot open relay reservation directory: %w", err)
	}
	defer root.Close()
	name := strconv.Itoa(port) + ".lock"
	// Separate atomic creation from opening an existing inode. Concurrent
	// OpenRoot O_CREATE opens can return ENOENT on macOS without O_EXCL.
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if errors.Is(err, os.ErrExist) {
		file, err = root.OpenFile(name, os.O_RDWR, 0)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot open relay port reservation: %w", err)
	}
	if err = lockPortFile(file); err != nil {
		file.Close()
		if errors.Is(err, errPortLocked) {
			return nil, fmt.Errorf("relay port %d is already reserved by another Zelport relay instance", port)
		}
		return nil, fmt.Errorf("cannot reserve relay port %d: %w", port, err)
	}
	return file, nil
}

var errPortLocked = errors.New("relay port locked")
