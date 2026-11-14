//go:build !windows

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

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

func dialDesktop(ctx context.Context, root string) (net.Conn, error) {
	paths := []string{
		filepath.Join(root, "ipc", "ipc.sock"),
		filepath.Join(os.TempDir(), "codex-ipc", fmt.Sprintf("ipc-%d.sock", os.Getuid())),
	}
	if real, err := filepath.EvalSymlinks(filepath.Join(root, "ipc")); err == nil {
		hash := sha256.Sum256([]byte(real))
		paths = append(paths, filepath.Join("/tmp", fmt.Sprintf("codex-ipc-%d", os.Getuid()), fmt.Sprintf("%x", hash), "ipc.sock"))
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Getuid()) {
			continue
		}
		if conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path); err == nil {
			return conn, nil
		}
	}
	return nil, errors.New("connexion locale Codex Desktop indisponible")
}

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
