package main

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"io"
	"mime/multipart"
	"net/http"
)

func newJob(r *http.Request, w http.ResponseWriter, logger *customLogger) error {
	jobID := uuid.New().String()
	jobLogger := newCustomLogger(logger, fmt.Sprintf("job %s: ", jobID))

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
	if err == nil {
		defer taskProcessor.Close()
		taskProcessor.SetLogger(jobLogger)
		// Delete multipart file before running command. Saves RAM (tmpfs)
		_ = formFile.Close()
		_ = r.MultipartForm.RemoveAll()
		if err = taskProcessor.Run(); err != nil {
			return fmt.Errorf("failed to process file in job %s: %v", jobID, err.Error())
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
	defer resp.Body.Close()
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
	// Send immich response back to client
	addHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		return fmt.Errorf("unable to forward response to client: %v", err)
	}

	return nil
}
