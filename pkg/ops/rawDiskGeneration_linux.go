//go:build linux
// +build linux

package ops

import (
	"golang.org/x/sys/unix"
)

// Linux-specific implementations of syscalls
func getFileFlags(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	_, err = unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
	return err
}

func setFileFlags(path string, flags uint32) error {
	fd, err := unix.Open(path, unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.IoctlSetInt(fd, unix.FS_IOC_SETFLAGS, int(flags))
}

func initModule(path string, params string) error {
	return unix.InitModule([]byte(params), "")
}

func finitModule(fd int, params string) error {
	return unix.FinitModule(fd, params, 0)
}

func deleteModule(name string) error {
	return unix.DeleteModule(name, 0)
}
