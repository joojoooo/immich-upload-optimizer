package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

var jobID int

func newJob(r *http.Request, w http.ResponseWriter, logger *customLogger) error {
	jobID++
	jobLogger := newCustomLogger(logger, fmt.Sprintf("job %d: ", jobID))

	formFile, formFileHeader, err := r.FormFile(filterFormKey)
	if err != nil {
		return fmt.Errorf("unable to read file in key %s from uploaded form data: %w", filterFormKey, err)
	}
	defer r.MultipartForm.RemoveAll()
	defer formFile.Close()

	jobLogger.Printf("download original: \"%s\" (%s)", formFileHeader.Filename, humanReadableSize(formFileHeader.Size))

	var originalHash string
	var newHash string
	uploadFile := formFile
	uploadFilename := formFileHeader.Filename
	uploadOriginal := true

	taskProcessor, err := NewTaskProcessorFromMultipart(formFile, formFileHeader)
	if err == nil && taskProcessor != nil {
		defer taskProcessor.Close()
		taskProcessor.SetLogger(jobLogger)
		// Delete multipart file before running command. Saves RAM (tmpfs)
		_ = formFile.Close()
		_ = r.MultipartForm.RemoveAll()
		if err = taskProcessor.Run(); err != nil {
			return fmt.Errorf("failed to process file in job %d: %v", jobID, err.Error())
		}
		if taskProcessor.OriginalSize <= taskProcessor.ProcessedSize {
			uploadFile = taskProcessor.OriginalFile
			_ = taskProcessor.CleanWorkDir() // Save RAM before upload (tmpfs)
		} else {
			uploadFile = taskProcessor.ProcessedFile
			uploadFilename = taskProcessor.ProcessedFilename
			uploadOriginal = false
			if originalHash, err = SHA1(taskProcessor.OriginalFile); err != nil {
				return fmt.Errorf("sha1: %w", err)
			}
			_ = taskProcessor.CleanOriginalFile() // Save RAM before upload (tmpfs)
		}
	}
	// Upload the original file or processed one if a task was found
	err = uploadUpstream(w, r, uploadFile, uploadFilename)
	if err != nil {
		jobLogger.Printf("upload upstream error: %s", err.Error())
		http.Error(w, "failed to process file, view logs for more info", http.StatusInternalServerError)
	}
	if uploadOriginal {
		jobLogger.Printf("uploaded original: \"%s\" (%s)", formFileHeader.Filename, humanReadableSize(formFileHeader.Size))
	} else {
		if newHash, err = SHA1(taskProcessor.ProcessedFile); err != nil {
			return fmt.Errorf("new sha1: %w", err)
		}
		addChecksums(newHash, originalHash)
		jobLogger.Printf("uploaded: \"%s\" (%s) <- (%s) \"%s\"", taskProcessor.ProcessedFilename, humanReadableSize(taskProcessor.ProcessedSize), humanReadableSize(taskProcessor.OriginalSize), taskProcessor.OriginalFilename)
	}

	return nil
}

func uploadUpstream(w http.ResponseWriter, r *http.Request, file io.ReadSeeker, name string) (err error) {
	pipeReader, pipeWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(pipeWriter)
	errChan := make(chan error, 1)
	defer close(errChan)
	// Prepare chunked request, this saves A LOT of RAM compared to building the whole buffer in RAM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer pipeWriter.Close()
		defer multipartWriter.Close()
		for key, values := range r.MultipartForm.Value {
			for _, value := range values {
				err = multipartWriter.WriteField(key, value)
				if err != nil {
					cancel()
					errChan <- fmt.Errorf("unable to create form data: %w", err)
					return
				}
			}
		}
		part, err := multipartWriter.CreateFormFile(filterFormKey, name)
		if err != nil {
			cancel()
			errChan <- fmt.Errorf("unable to create form data: %w", err)
			return
		}
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			cancel()
			errChan <- fmt.Errorf("unable to seek beginning of file: %w", err)
			return
		}
		_, err = io.Copy(part, file)
		if err != nil {
			cancel()
			errChan <- fmt.Errorf("unable to write file in form field: %w", err)
			return
		}
		err = multipartWriter.Close()
		if err != nil {
			cancel()
			errChan <- fmt.Errorf("unable to finish form data: %w", err)
			return
		}
		errChan <- nil
	}()
	req, err := http.NewRequestWithContext(ctx, "POST", upstreamURL+r.URL.String(), pipeReader)
	if err != nil {
		return fmt.Errorf("unable to create POST request: %w", err)
	}
	req.Header = r.Header
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	// Send the request to the upstream server
	resp, err := getHTTPclient().Do(req)
	if err != nil {
		select {
		case chErr := <-errChan:
			if err != nil {
				return fmt.Errorf("error writing data to pipe: %v: %v", err, chErr)
			}
		default:
		}
		return fmt.Errorf("unable to POST: %w", err)
	}

	defer resp.Body.Close()
	// Extract asset ID from response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}
	var responseData struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bodyBytes, &responseData); err != nil {
		return fmt.Errorf("failed to parse upstream response JSON: %w", err)
	}
	if responseData.ID == "" {
		return fmt.Errorf("no asset ID found in upstream response")
	}
	// Tag the asset
	if len(tagIDs) > 0 {
		if err := tagAsset(ctx, upstreamURL, responseData.ID, tagIDs, r); err != nil {
			return err
		}
	}
	// Send immich response back to client
	setHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("unable to forward response to client: %v", err)
	}

	return nil
}

// tagAsset makes an API call to tag an asset with the specified tag IDs.
func tagAsset(ctx context.Context, upstreamURL string, assetID string, tagIDs []string, originalReq *http.Request) error {
	// Prepare JSON body for tagging
	tagBody := map[string][]string{"assetIds": {assetID}, "tagIds": tagIDs}
	tagBodyBytes, err := json.Marshal(tagBody)
	if err != nil {
		return fmt.Errorf("failed to marshal tag JSON body: %w", err)
	}

	// Create tag request
	tagURL := fmt.Sprintf("%s/api/tags/assets", upstreamURL)
	tagReq, err := http.NewRequestWithContext(ctx, "PUT", tagURL, bytes.NewReader(tagBodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create tag request: %w", err)
	}

	// Set headers for authentication and content type
	tagReq.Header.Set("Content-Type", "application/json")
	// Copy cookies from original request to reuse session
	for _, cookie := range originalReq.Cookies() {
		tagReq.AddCookie(cookie)
	}
	// Copy other headers that might be relevant for authentication
	if apiKey := originalReq.Header.Get("x-api-key"); apiKey != "" {
		tagReq.Header.Set("x-api-key", apiKey)
	}

	// Send tag request
	tagResp, err := getHTTPclient().Do(tagReq)
	if err != nil {
		return fmt.Errorf("failed to add tag to asset: %w", err)
	}
	defer tagResp.Body.Close()

	// Check response status
	if tagResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(tagResp.Body)
		return fmt.Errorf("tag request failed with status %s: %s", tagResp.Status, string(body))
	}

	return nil
}
