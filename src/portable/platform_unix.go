//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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

func packagedCodex(_ context.Context) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	if path := "/Applications/Codex.app/Contents/Resources/codex"; isCodexExecutable(path) {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex")
		if isCodexExecutable(path) {
			return path
		}
	}
	return ""
}

func downloadsFolder() (string, error) {
	if runtime.GOOS == "linux" {
		if output, err := exec.Command("xdg-user-dir", "DOWNLOAD").Output(); err == nil {
			if folder := strings.TrimSpace(string(output)); filepath.IsAbs(folder) {
				return folder, nil
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Downloads"), nil
}
