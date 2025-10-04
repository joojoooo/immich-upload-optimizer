package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// syncChecksumTypes lists the stream event types that carry an asset checksum we need to rewrite.
var syncChecksumTypes = map[string]struct{}{
	"AssetV1":                {},
	"PartnerAssetV1":         {},
	"PartnerAssetBackfillV1": {},
	"AlbumAssetCreateV1":     {},
	"AlbumAssetUpdateV1":     {},
	"AlbumAssetBackfillV1":   {},
}

func proxySyncStream(w http.ResponseWriter, r *http.Request, logger *customLogger) error {
	req, err := http.NewRequest(r.Method, upstreamURL+r.URL.String(), r.Body)
	if err != nil {
		return err
	}
	req.Header = r.Header.Clone()
	req.ContentLength = r.ContentLength
	if len(r.TransferEncoding) > 0 {
		req.TransferEncoding = append([]string(nil), r.TransferEncoding...)
	}
	if r.GetBody != nil {
		req.GetBody = r.GetBody
	}

	resp, err := getHTTPclient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	setHeaders(w.Header(), resp.Header)
	w.Header().Del("Content-Length")

	w.WriteHeader(resp.StatusCode)

	bodyReader, bodyWriter := getBodyWriterReaderHTTP(&w, resp)
	defer bodyReader.Close()
	defer bodyWriter.Close()

	scanner := bufio.NewScanner(bodyReader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {

		line := append([]byte(nil), scanner.Bytes()...)

		processed, handled, changed, eventType, missing := rewriteSyncStreamLine(line)

		if handled {

			if changed {

				logger.Printf("sync stream: rewrote checksum(s) for %s", eventType)

			} else if len(missing) > 0 {

				logger.Printf("sync stream: missing checksum mapping for %s checksum=%v", eventType, missing)

			}

		} else if eventType != "" {

			logger.Printf("sync stream: unhandled event %s", eventType)

		}

		if len(processed) > 0 {

			if _, err = bodyWriter.Write(processed); err != nil {

				return err

			}

		}

		if _, err = bodyWriter.Write([]byte("\n")); err != nil {
			return err

		}

		if flusher, ok := bodyWriter.(interface{ Flush() error }); ok {

			_ = flusher.Flush()

		}

		if f, ok := w.(http.Flusher); ok {

			f.Flush()

		}

	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	return nil
}

// rewriteSyncStreamLine updates a single JSON line when it contains a supported asset event.
func rewriteSyncStreamLine(line []byte) ([]byte, bool, bool, string, []string) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return line, false, false, "", nil
	}

	var event map[string]any
	if err := json.Unmarshal(trimmed, &event); err != nil {
		return line, false, false, "", nil
	}

	eventType, handled, changed, missing := rewriteSyncStreamEvent(event)

	if !handled {
		return line, false, false, eventType, missing
	}
	if !changed {
		return line, true, false, eventType, missing
	}

	updated, err := json.Marshal(event)
	if err != nil {
		return line, true, false, eventType, missing
	}
	return updated, true, true, eventType, missing
}

// rewriteSyncStreamEvent rewrites known payloads and reports missing checksum mappings.
func rewriteSyncStreamEvent(event map[string]any) (string, bool, bool, []string) {
	eventType, _ := event["type"].(string)
	if _, ok := syncChecksumTypes[eventType]; !ok {
		return eventType, false, false, nil
	}
	payload, _ := event["data"]
	var missing []string
	changed := rewriteChecksumPayload(payload, &missing)
	return eventType, true, changed, missing
}

func rewriteChecksumPayload(payload any, missing *[]string) bool {
	switch v := payload.(type) {
	case map[string]any:
		return rewriteChecksumMap(v, missing)
	case []any:
		changed := false
		for _, item := range v {
			if rewriteChecksumPayload(item, missing) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func rewriteChecksumMap(m map[string]any, missing *[]string) bool {
	changed := false
	if checksum, ok := m["checksum"].(string); ok {
		mapLock.RLock()
		original, exists := fakeToOriginalChecksum[checksum]
		mapLock.RUnlock()
		if exists {
			m["checksum"] = original
			changed = true
		} else if missing != nil {
			*missing = append(*missing, checksum)
		}
	}
	for key, value := range m {
		if key == "checksum" {
			continue
		}
		if rewriteChecksumPayload(value, missing) {
			changed = true
		}
	}
	return changed
}
