package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
)

func humanReadableSize(size int64) string {
	const (
		_  = iota // ignore first value by assigning to blank identifier
		KB = 1 << (10 * iota)
		MB
		GB
		TB
	)

	switch {
	case size >= TB:
		return fmt.Sprintf("%.2f TB", float64(size)/TB)
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}

func isValidFilename(s string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	return re.MatchString(s)
}

func printVersion() string {
	return fmt.Sprintf("immich-upload-optimizer %s, commit %s, built at %s", version, commit, date)
}

func validateInput() {
	if upstreamURL == "" {
		log.Fatal("the -upstream flag is required")
	}

	var err error
	remote, err = url.Parse(upstreamURL)
	if err != nil {
		log.Fatalf("invalid upstream URL: %v", err)
	}

	if configFile == "" {
		log.Fatal("the -tasks_file flag is required")
	}

	config, err = NewConfig(&configFile)
	if err != nil {
		log.Fatalf("error loading config file: %v", err)
	}
}

func removeAllContents(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if info.IsDir() {
			return os.RemoveAll(path)
		}
		return os.Remove(path)
	})
}
