package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"slices"
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
var originalToFakeChecksum map[string]string

func initChecksums() {
	fakeToOriginalChecksum = make(map[string]string)
	originalToFakeChecksum = make(map[string]string)
	file, err := os.OpenFile(checksumsFile, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		kv := strings.Split(scanner.Text(), ",")
		setChecksumPair(kv[0], kv[1])
	}
	if err := scanner.Err(); err != nil {
		fmt.Println(red("Error reading csv: %v", err))
	}
}

// setChecksumPair ensures a fake<->original 1:1 relationship. Caller must hold mapLock.
func setChecksumPair(fake, original string) bool {
	if existingOriginal, ok := fakeToOriginalChecksum[fake]; ok && existingOriginal == original {
		fmt.Println(magenta("Duplicate checksum pair: %s <-> %s", fake, original))
		return false
	}
	if oldOriginal, ok := fakeToOriginalChecksum[fake]; ok && oldOriginal != original {
		fmt.Println(red("Duplicate fake checksum: %s -> %s , %s", fake, oldOriginal, original))
		delete(originalToFakeChecksum, oldOriginal)
	}
	if oldFake, ok := originalToFakeChecksum[original]; ok && oldFake != fake {
		fmt.Println(red("Duplicate orig checksum: %s -> %s , %s", original, oldFake, fake))
		delete(fakeToOriginalChecksum, oldFake)
	}
	fakeToOriginalChecksum[fake] = original
	originalToFakeChecksum[original] = fake
	return true
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
	if downloadJpgFromJxl || downloadJpgFromAvif {
		if n, ok := asset["originalFileName"]; ok {
			if originalFileName, ok := n.(string); ok {
				extension := strings.ToLower(path.Ext(originalFileName))
				if (downloadJpgFromJxl && extension == ".jxl") || (downloadJpgFromAvif && extension == ".avif") {
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

type bulkUploadCheckItem struct {
	ID       string `json:"id"`
	Checksum string `json:"checksum"`
}

type bulkUploadCheckRequest struct {
	Assets []bulkUploadCheckItem `json:"assets"`
}

type bulkUploadCheckResult struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	AssetID string `json:"assetId"`
}

type bulkUploadCheckResponse struct {
	Results []bulkUploadCheckResult `json:"results"`
}

type assetMediaResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Sync stream entries carrying an asset whose checksum must be replaced.
// Immich v3 emits the V2 variants, older servers the V1 ones.
var checksumRewriteStreamTypes = []string{
	"AssetV1", "AssetV2",
	"AlbumAssetCreateV1", "AlbumAssetCreateV2",
	"AlbumAssetUpdateV1", "AlbumAssetUpdateV2",
	"AlbumAssetBackfillV1", "AlbumAssetBackfillV2",
	"PartnerAssetV1", "PartnerAssetV2",
	"PartnerAssetBackfillV1", "PartnerAssetBackfillV2",
}

func replaceBulkUploadCheck(w http.ResponseWriter, r *http.Request, logger *customLogger) error {
	logger.SetErrPrefix("bulk-upload-check")
	var err error
	var bodyBytes []byte
	if bodyBytes, err = io.ReadAll(r.Body); logger.Error(err, "read body") {
		return err
	}
	var checkReq bulkUploadCheckRequest
	if err = json.Unmarshal(bodyBytes, &checkReq); logger.Error(err, "json unmarshal") {
		return err
	}
	mapLock.RLock()
	for i, asset := range checkReq.Assets {
		key := asset.Checksum
		if raw, err := hex.DecodeString(key); err == nil && len(raw) == sha1.Size {
			key = base64.StdEncoding.EncodeToString(raw)
		}
		if fake, ok := originalToFakeChecksum[key]; ok {
			checkReq.Assets[i].Checksum = fake
		}
	}
	mapLock.RUnlock()
	if bodyBytes, err = json.Marshal(checkReq); logger.Error(err, "json marshal") {
		return err
	}
	var req *http.Request
	if req, err = http.NewRequest(r.Method, upstreamURL+r.URL.String(), bytes.NewReader(bodyBytes)); logger.Error(err, "new request") {
		return err
	}
	req.Header = r.Header
	var resp *http.Response
	if resp, err = getHTTPclient().Do(req); logger.Error(err, "getHTTPclient.Do") {
		return err
	}
	defer resp.Body.Close()
	setHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(w, resp.Body); logger.Error(err, "resp write") {
		return err
	}
	return nil
}

func getChecksumReplacer(w http.ResponseWriter, r *http.Request, logger *customLogger) *Replacer {
	if isStreamSync(r) {
		return &Replacer{w, r, logger, TypeStream}
	}
	if isFullSync(r) {
		return &Replacer{w, r, logger, TypeFull}
	}
	if isDeltaSync(r) {
		return &Replacer{w, r, logger, TypeDelta}
	}
	/*
		Since immich server v1.133.1
		- Albums don't come with assets on the web (?withoutAssets=true by default) but still do for the app
		- Buckets don't hold assets anymore
	*/
	if isAlbum(r) {
		return &Replacer{w, r, logger, TypeAlbum}
	}
	/*
		if isBucket(r) {
			return &Replacer{w, r, logger, TypeBucket}
		}
	*/
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
	TypeStream
)

func (replacer Replacer) Replace() (err error) {
	w, r, logger := replacer.w, replacer.r, replacer.logger
	var req *http.Request
	var resp *http.Response
	if req, err = http.NewRequest(r.Method, upstreamURL+r.URL.String(), nil); logger.Error(err, "new request") {
		return
	}
	req.Header = r.Header
	req.Body = r.Body
	if resp, err = getHTTPclient().Do(req); logger.Error(err, "getHTTPclient.Do") {
		return
	}
	defer resp.Body.Close()
	bodyReader, bodyWriter := getBodyWriterReaderHTTP(&w, resp)
	defer bodyReader.Close()
	defer bodyWriter.Close()
	var jsonBuf []byte
	if jsonBuf, err = io.ReadAll(bodyReader); logger.Error(err, "resp read") {
		return
	}
	if resp.StatusCode == http.StatusOK {
		assetsKey := "assets"
		switch replacer.typeId {
		case TypeStream:
			fixedJsonBuf := make([]byte, len(jsonBuf)+1)
			fixedJsonBuf[0] = '['
			copy(fixedJsonBuf[1:], replaceAllBytes(jsonBuf, []byte("\n"), []byte(",")))
			fixedJsonBuf[len(fixedJsonBuf)-1] = ']'
			var streams []any
			if err = json.Unmarshal(fixedJsonBuf, &streams); logger.Error(err, "json unmarshal") {
				return
			}
			for _, value := range streams {
				if v, ok := value.(map[string]any); ok {
					if t, ok := v["type"].(string); ok && !slices.Contains(checksumRewriteStreamTypes, t) {
						continue
					}
					if asset, ok := v["data"].(map[string]any); ok {
						mapLock.RLock()
						Asset(asset).toOriginalAsset()
						mapLock.RUnlock()
					}
				}
			}
			if jsonBuf, err = json.Marshal(streams); logger.Error(err, "json marshal") {
				return
			}
			if len(jsonBuf) > 0 {
				jsonBuf = jsonBuf[1:]
				jsonBuf[len(jsonBuf)-1] = '\n'
			}
			replaceAllBytes(jsonBuf, []byte("},{"), []byte("}\n{"))
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
	if !slices.Contains([]string{"gzip", "br"}, resp.Header.Get("Content-Encoding")) {
		w.Header().Set("Content-Length", strconv.Itoa(len(jsonBuf)))
	}
	w.WriteHeader(resp.StatusCode)
	if _, err = bodyWriter.Write(jsonBuf); logger.Error(err, "resp write") {
		return
	}
	return
}
