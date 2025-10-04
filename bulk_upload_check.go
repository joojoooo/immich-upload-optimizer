package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

type assetBulkUploadCheckPayload struct {
	Assets []assetBulkUploadCheckItem `json:"assets"`
}

type assetBulkUploadCheckItem struct {
	Checksum string `json:"checksum"`
	ID       string `json:"id"`
}

func rewriteBulkUploadCheckRequest(r *http.Request) (int, error) {
	if r.Body == nil {
		return 0, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return 0, err
	}
	_ = r.Body.Close()

	if len(body) == 0 {
		reader := bytes.NewReader(body)
		r.Body = io.NopCloser(reader)
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		r.ContentLength = int64(len(body))
		r.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return 0, nil
	}

	var payload assetBulkUploadCheckPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		reader := bytes.NewReader(body)
		r.Body = io.NopCloser(reader)
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		r.ContentLength = int64(len(body))
		r.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return 0, err
	}

	modified := 0
	mapLock.RLock()
	for idx, asset := range payload.Assets {
		if fake, ok := originalToFakeChecksum[asset.Checksum]; ok {
			payload.Assets[idx].Checksum = fake
			modified++
		}
	}
	mapLock.RUnlock()

	newBody := body
	if modified > 0 {
		updated, err := json.Marshal(payload)
		if err != nil {
			reader := bytes.NewReader(body)
			r.Body = io.NopCloser(reader)
			r.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(body)), nil
			}
			r.ContentLength = int64(len(body))
			r.Header.Set("Content-Length", strconv.Itoa(len(body)))
			return 0, err
		}
		newBody = updated
	}

	reader := bytes.NewReader(newBody)
	r.Body = io.NopCloser(reader)
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(newBody)), nil
	}
	r.ContentLength = int64(len(newBody))
	r.Header.Set("Content-Length", strconv.Itoa(len(newBody)))

	return modified, nil
}
