package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
var mapLock sync.RWMutex
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

func addChecksums(fake, original string) {
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

// toOriginalAsset: Must acquire mapLock.RLock() before calling
func (asset Asset) toOriginalAsset() {
	if c, ok := asset["checksum"]; ok {
		if checksum, ok := c.(string); ok {
			if original, ok := fakeToOriginalChecksum[checksum]; ok {
				//fmt.Printf("checksum: %s -> %s\n", checksum, original)
				asset["checksum"] = original
			}
		}
	}
}

func getChecksumReplacer(r *http.Request) checksumReplacer {
	if isDeltaSync(r) {
		return deltaChecksumReplacer{}
	}
	if isFullSync(r) {
		return fullChecksumReplacer{}
	}
	if isAlbum(r) {
		return albumChecksumReplacer{}
	}
	return nil
}

type checksumReplacer interface {
	Replace(w http.ResponseWriter, r *http.Request, logger *customLogger) error
}

type albumChecksumReplacer struct{}
type deltaChecksumReplacer struct{}
type fullChecksumReplacer struct{}

func (replacer albumChecksumReplacer) Replace(w http.ResponseWriter, r *http.Request, logger *customLogger) (err error) {
	return replacerWithAssetsKey(w, r, logger, "assets")
}

func (replacer deltaChecksumReplacer) Replace(w http.ResponseWriter, r *http.Request, logger *customLogger) (err error) {
	return replacerWithAssetsKey(w, r, logger, "upserted")
}

func (replacer fullChecksumReplacer) Replace(w http.ResponseWriter, r *http.Request, logger *customLogger) (err error) {
	jsonBuf, err := replacerDoRequest(w, r, logger)
	if err != nil {
		return
	}
	var assets []Asset
	if err = json.Unmarshal(jsonBuf, &assets); logger.Error(err, "json unmarshal") {
		return
	}
	mapLock.RLock()
	for _, asset := range assets {
		asset.toOriginalAsset()
	}
	mapLock.RUnlock()
	if jsonBuf, err = json.Marshal(assets); logger.Error(err, "json marshal") {
		return
	}
	if _, err = w.Write(jsonBuf); logger.Error(err, "resp write") {
		return
	}
	return nil
}

func replacerDoRequest(w http.ResponseWriter, r *http.Request, logger *customLogger) (jsonBuf []byte, err error) {
	var req *http.Request
	var resp *http.Response
	if req, err = http.NewRequest(r.Method, upstreamURL+r.URL.String(), nil); logger.Error(err, "new POST") {
		return
	}
	req.Header = r.Header
	req.Body = r.Body
	if resp, err = getHTTPclient().Do(req); logger.Error(err, "getHTTPclient.Do") {
		return
	}
	defer resp.Body.Close()
	if jsonBuf, err = io.ReadAll(resp.Body); logger.Error(err, "resp read") {
		return
	}
	addHeaders(w.Header(), resp.Header)
	return
}

func replacerWithAssetsKey(w http.ResponseWriter, r *http.Request, logger *customLogger, assetsKey string) (err error) {
	jsonBuf, err := replacerDoRequest(w, r, logger)
	if err != nil {
		return
	}
	var assetsMap map[string]any
	if err = json.Unmarshal(jsonBuf, &assetsMap); logger.Error(err, "json unmarshal") {
		return
	}
	for key, value := range assetsMap {
		if key != assetsKey {
			continue
		}
		if assets, ok := value.([]any); ok {
			mapLock.RLock()
			for _, a := range assets {
				if asset, ok := a.(map[string]any); ok {
					Asset(asset).toOriginalAsset()
				}
			}
			mapLock.RUnlock()
		}
		break
	}
	if jsonBuf, err = json.Marshal(assetsMap); logger.Error(err, "json marshal") {
		return
	}
	if _, err = w.Write(jsonBuf); logger.Error(err, "resp write") {
		return
	}
	return
}
