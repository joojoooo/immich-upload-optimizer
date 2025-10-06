package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/andybalholm/brotli"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var filterFormKey = "assetData"

func isAssetsUpload(r *http.Request) bool {
	return r.Method == "POST" && r.URL.Path == "/api/assets" && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
}

func isStreamSync(r *http.Request) bool {
	return r.Method == "POST" && r.URL.Path == "/api/sync/stream"
}

func isFullSync(r *http.Request) bool {
	return r.Method == "POST" && r.URL.Path == "/api/sync/full-sync"
}

func isDeltaSync(r *http.Request) bool {
	return r.Method == "POST" && r.URL.Path == "/api/sync/delta-sync"
}

func isAlbum(r *http.Request) bool {
	re := regexp.MustCompile(`^/api/albums/[a-z0-9]{8}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{12}$`)
	return r.Method == "GET" && re.MatchString(r.URL.Path)
}

func isBucket(r *http.Request) bool {
	return r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/timeline/bucket")
}

func isAssetView(r *http.Request) bool {
	re := regexp.MustCompile(`^/api/assets/[a-z0-9]{8}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{12}$`)
	return r.Method == "GET" && re.MatchString(r.URL.Path)
}

func isOriginalDownloadPath(r *http.Request) (bool, []string) {
	re := regexp.MustCompile(`^/api/assets/([a-z0-9]{8}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{12})/original$`)
	matches := re.FindStringSubmatch(r.URL.String())
	return r.Method == "GET" && len(matches) == 2, matches
}

func replaceAllBytes(byteSlice []byte, old []byte, new []byte) []byte {
	oldLen := len(old)
	newLen := len(new)
	if newLen > oldLen {
		return byteSlice
	}
	offset := 0
	for {
		i := bytes.Index(byteSlice[offset:], old)
		if i == -1 {
			break
		}
		offset += i
		copy(byteSlice[offset:], new)
		offset += newLen
	}
	return byteSlice
}

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

func getHTTPclient() (client *http.Client) {
	if DevMITMproxy {
		client = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyUrl)}}
	} else {
		client = &http.Client{}
	}
	return
}

func setHeaders(h1, h2 http.Header) {
	deleteAllHeaders(h1)
	for key, values := range h2 {
		h1[key] = values
	}
}

func deleteAllHeaders(h http.Header) {
	for key := range h {
		h.Del(key)
	}
}

func webSocketSafeHeader(header http.Header) http.Header {
	header = header.Clone()
	for _, v := range []string{"Upgrade", "Connection", "Sec-Websocket-Key", "Sec-Websocket-Version", "Sec-Websocket-Extensions", "Sec-Websocket-Protocol"} {
		header.Del(v)
	}
	return header
}

type nopWriteCloser struct {
	io.Writer
}

func (nwc nopWriteCloser) Close() error {
	return nil
}

func NopWriteCloser(w io.Writer) io.WriteCloser {
	return nopWriteCloser{w}
}

func getBodyWriterReaderHTTP(w *http.ResponseWriter, resp *http.Response) (bodyReader io.ReadCloser, bodyWriter io.WriteCloser) {
	var err error
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		if bodyReader, err = gzip.NewReader(resp.Body); err != nil {
			break
		}
		if w != nil {
			bodyWriter = gzip.NewWriter(*w)
		}
		return
	case "br":
		bodyReader = io.NopCloser(brotli.NewReader(resp.Body))
		if w != nil {
			bodyWriter = brotli.NewWriter(*w)
		}
		return
	}
	bodyReader = io.NopCloser(resp.Body)
	if w != nil {
		bodyWriter = NopWriteCloser(*w)
	}
	return
}
