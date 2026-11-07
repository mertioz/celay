package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var reservedFileName = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[1-9¹²³]|CONIN\$|CONOUT\$)$`)

func saveDiscussion(ctx context.Context, root string, thread discussion) (string, error) {
	data, err := exportDiscussion(ctx, root, thread)
	if err != nil {
		return "", err
	}
	folder, err := downloadsFolder()
	if err != nil {
		return "", err
	}
	return writeMarkdown(folder, markdownFilename(thread.Title), data)
}

func writeMarkdown(folder, filename string, data []byte) (string, error) {
	if err := os.MkdirAll(folder, 0700); err != nil {
		return "", err
	}
	name := strings.TrimSuffix(filename, ".md")
	device, _, _ := strings.Cut(name, ".")
	if reservedFileName.MatchString(strings.TrimSpace(device)) {
		name = "Celay - " + name
	}
	for copy := 1; ; copy++ {
		base := name
		if copy > 1 {
			base = fmt.Sprintf("%s (%d)", name, copy)
		}
		path := filepath.Join(folder, base+".md")
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, writeErr := file.Write(data)
		if err := errors.Join(writeErr, file.Close()); err != nil {
			os.Remove(path)
			return "", err
		}
		return path, nil
	}
}
