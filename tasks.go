package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
)

type TaskProcessor struct {
	Task              *Task
	OriginalFile      *os.File
	OriginalFilename  string
	OriginalExtension string
	OriginalSize      int64

	tempOriginalFilePath string

	ProcessedFile      *os.File
	ProcessedFilename  string
	ProcessedExtension string
	ProcessedSize      int64

	tempWorkDir string

	logger *customLogger
}

func NewTaskProcessorFromMultipart(file multipart.File, header *multipart.FileHeader) (*TaskProcessor, error) {
	originalExtension := path.Ext(header.Filename)
	if !isValidFilename(originalExtension) {
		return nil, fmt.Errorf("invalid file extension: %s", originalExtension)
	}

	// Must have a task, passthrough the request to immich otherwise
	checkExt := strings.ToLower(strings.TrimPrefix(originalExtension, "."))
	var task *Task
	for _, t := range config.Tasks {
		if slices.Contains(t.Extensions, checkExt) {
			task = t
			break
		}
	}
	if task == nil {
		return nil, fmt.Errorf("no task found for file extension .%s", checkExt)
	}

	if header.Size < task.MinFilesizeBytes {
		return nil, fmt.Errorf("file size is smaller than minimum: %d < %d", header.Size, task.MinFilesizeBytes)
	}

	originalFile, err := os.CreateTemp("", "upload-*"+originalExtension)
	if err != nil {
		return nil, fmt.Errorf("unable to create temp file: %w", err)
	}

	_, err = io.Copy(originalFile, file)
	if err != nil {
		return nil, fmt.Errorf("unable to write temp file: %w", err)
	}

	return &TaskProcessor{
		Task:                 task,
		OriginalFile:         originalFile,
		OriginalFilename:     header.Filename,
		OriginalExtension:    originalExtension,
		OriginalSize:         header.Size,
		tempOriginalFilePath: originalFile.Name(),
	}, nil
}

func (tp *TaskProcessor) SetLogger(logger *customLogger) {
	tp.logger = logger
}

func (tp *TaskProcessor) logf(str string, args ...interface{}) {
	if tp.logger != nil {
		tp.logger.Printf(str, args...)
	}
}

func (tp *TaskProcessor) Close() error {
	_ = tp.CleanOriginalFile()
	return tp.CleanWorkDir()
}

func (tp *TaskProcessor) CleanOriginalFile() (err error) {
	if tp.OriginalFile != nil {
		err = tp.OriginalFile.Close()
		if err != nil {
			tp.logf("unable to close original file: %v", err)
		}
		tp.OriginalFile = nil
	}

	if tp.tempOriginalFilePath != "" {
		err = os.Remove(tp.tempOriginalFilePath)
		if err != nil {
			tp.logf("unable to remove temp file: %v", err)
		}
		tp.tempOriginalFilePath = ""
	}
	return
}

func (tp *TaskProcessor) CleanWorkDir() error {
	if tp.tempWorkDir == "" {
		return nil
	}
	err := os.RemoveAll(tp.tempWorkDir)
	if err != nil {
		tp.logf("unable to clean temp folder: %v", err)
	}
	tp.tempWorkDir = ""
	return err
}

func (tp *TaskProcessor) Run() error {
	// Limit the number of concurrent tasks running
	semaphore <- struct{}{}
	defer func() { <-semaphore }()
	var err error

	tp.tempWorkDir, err = os.MkdirTemp("", "processing-*")
	if err != nil {
		return fmt.Errorf("unable to create temp folder: %w", err)
	}

	basename := path.Base(tp.tempOriginalFilePath)
	extension := path.Ext(basename)
	values := map[string]string{
		"result_folder": tp.tempWorkDir,
		"folder":        path.Dir(tp.tempOriginalFilePath),
		"name":          strings.TrimSuffix(basename, extension),
		"extension":     strings.TrimPrefix(extension, "."),
	}

	var cmdLine bytes.Buffer
	err = tp.Task.CommandTemplate.Execute(&cmdLine, values)
	if err != nil {
		return fmt.Errorf("unable to generate command to be Run: %w", err)
	}
	tp.logf("running task: %s: %s", tp.Task.Name, cmdLine.String())
	cmd := exec.Command("sh", "-c", cmdLine.String())
	cmd.Dir = path.Dir(configFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w while running command:\n%s\nOutput:\n%s", err, cmdLine.String(), string(output))
	}

	files, err := os.ReadDir(tp.tempWorkDir)
	if err != nil {
		return fmt.Errorf("unable to read temp directory: %w", err)
	}

	if len(files) != 1 {
		return fmt.Errorf("unexpected number of files in temp directory: %d", len(files))
	}

	processedFilePath := path.Join(tp.tempWorkDir, files[0].Name())
	tp.ProcessedFile, err = os.Open(processedFilePath)
	if err != nil {
		return fmt.Errorf("unable to open temp file: %w", err)
	}
	stat, err := os.Stat(processedFilePath)
	if err != nil {
		err = fmt.Errorf("unable to get file size: %w", err)
	}
	tp.ProcessedSize = stat.Size()
	tp.ProcessedExtension = path.Ext(processedFilePath)
	tp.ProcessedFilename = strings.TrimSuffix(tp.OriginalFilename, tp.OriginalExtension) + tp.ProcessedExtension

	return nil
}
