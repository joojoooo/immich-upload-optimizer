package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

func SHA1(file io.ReadSeeker) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("unable to seek beginning of file: %w", err)
	}
	hasher := sha1.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("could not copy file content to hasher: %v", err)
	}
	return base64.StdEncoding.EncodeToString(hasher.Sum(nil)), nil
}

var ChecksumsCsvPath string
var mapLock sync.Mutex
var fakeToOriginalChecksum map[string]string

func init() {
	fakeToOriginalChecksum = make(map[string]string)
	ChecksumsCsvPath = "checksums.csv"
	file, err := os.OpenFile(ChecksumsCsvPath, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		kv := strings.Split(scanner.Text(), ",")
		fakeToOriginalChecksum[kv[0]] = kv[1]
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading csv:", err)
	}
}

func GetOriginalChecksum(fake string) (string, bool) {
	mapLock.Lock()
	defer mapLock.Unlock()
	original, ok := fakeToOriginalChecksum[fake]
	return original, ok
}

func AddChecksums(fake, original string) {
	go func() {
		mapLock.Lock()
		fakeToOriginalChecksum[fake] = original
		mapLock.Unlock()
		_ = appendToCSV(fake, original)
	}()
}

func appendToCSV(key, value string) error {
	file, err := os.OpenFile(ChecksumsCsvPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.WriteString(file, key+","+value+"\n"); err != nil {
		return err
	}
	return nil
}

type Asset map[string]any

func (asset Asset) ToOriginalAsset() {
	if c, ok := asset["checksum"]; ok {
		if checksum, ok := c.(string); ok {
			if original, ok := GetOriginalChecksum(checksum); ok {
				asset["checksum"] = original
			}
		}
	}
}
