package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
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

var mapLock sync.RWMutex
var fakeToOriginalChecksum map[string]string

func initChecksums() {
	fakeToOriginalChecksum = make(map[string]string)
	file, err := os.OpenFile(checksumsFile, os.O_CREATE|os.O_RDONLY, 0644)
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
	file, err := os.OpenFile(checksumsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
	if downloadJpgFromJxl {
		if n, ok := asset["originalFileName"]; ok {
			if originalFileName, ok := n.(string); ok {
				if strings.ToLower(path.Ext(originalFileName)) == ".jxl" {
					asset["originalFileName"] = originalFileName + ".jpg"
				}
			}
		}
	}
	if c, ok := asset["checksum"]; ok {
		if checksum, ok := c.(string); ok {
			if original, ok := fakeToOriginalChecksum[checksum]; ok {
				//fmt.Printf("checksum: %s -> %s\n", checksum, original)
				asset["checksum"] = original
			}
		}
	}
}

func getChecksumReplacer(w http.ResponseWriter, r *http.Request, logger *customLogger) *Replacer {
	if isDeltaSync(r) {
		return &Replacer{w, r, logger, TypeDelta}
	}
	if isFullSync(r) {
		return &Replacer{w, r, logger, TypeFull}
	}
	if isAlbum(r) {
		return &Replacer{w, r, logger, TypeAlbum}
	}
	if isBucket(r) {
		return &Replacer{w, r, logger, TypeBucket}
	}
	if isAssetView(r) {
		return &Replacer{w, r, logger, TypeAssetView}
	}
	return nil
}

type Replacer struct {
	w      http.ResponseWriter
	r      *http.Request
	logger *customLogger
	typeId int
}

const (
	TypeAlbum = iota
	TypeDelta
	TypeFull
	TypeBucket
	TypeAssetView
)

func (replacer Replacer) Replace() (err error) {
	w, r, logger := replacer.w, replacer.r, replacer.logger
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
	var jsonBuf []byte
	if jsonBuf, err = io.ReadAll(resp.Body); logger.Error(err, "resp read") {
		return
	}
	if resp.StatusCode == http.StatusOK {
		assetsKey := "assets"
		switch replacer.typeId {
		case TypeDelta:
			assetsKey = "upserted"
			fallthrough
		case TypeAlbum:
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
		case TypeBucket:
			fallthrough
		case TypeFull:
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
		case TypeAssetView:
			var asset Asset
			if err = json.Unmarshal(jsonBuf, &asset); logger.Error(err, "json unmarshal") {
				return
			}
			mapLock.RLock()
			asset.toOriginalAsset()
			mapLock.RUnlock()
			if jsonBuf, err = json.Marshal(asset); logger.Error(err, "json marshal") {
				return
			}
		default:
			err = errors.New("invalid replacer type")
			return
		}
	}
	setHeaders(w.Header(), resp.Header)
	w.Header().Set("Content-Length", strconv.Itoa(len(jsonBuf)))
	w.WriteHeader(resp.StatusCode)
	if _, err = w.Write(jsonBuf); logger.Error(err, "resp write") {
		return
	}
	return
}
