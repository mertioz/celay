//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

func lockFile(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func hideConsole(_ *exec.Cmd) {}

func packagedCodex(_ context.Context) string { return "" }
