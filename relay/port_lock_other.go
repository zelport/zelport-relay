//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package relay

import (
	"errors"
	"os"
)

func lockPortFile(file *os.File) error {
	return errors.New("relay port reservations are not supported on this operating system")
}
