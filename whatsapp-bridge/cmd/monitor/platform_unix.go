//go:build linux || darwin

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func privateDefaults()                  { syscall.Umask(0o077) }
func privateConsole() (*os.File, error) { return os.OpenFile("/dev/tty", os.O_WRONLY, 0) }
func secureStore(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return errors.New("cannot create private monitor directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return errors.New("monitor directory must be a real owner-only directory")
	}
	for _, name := range []string{"session.db", "messages.db"} {
		path := filepath.Join(dir, name)
		if st, err := os.Lstat(path); err == nil {
			if !st.Mode().IsRegular() || st.Mode().Perm() != 0o600 || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
				return errors.New("monitor databases must be real owner-only files")
			}
		} else if !os.IsNotExist(err) {
			return errors.New("cannot inspect monitor database")
		}
	}
	return nil
}
