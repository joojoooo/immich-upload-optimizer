package main

import (
	"fmt"
	"regexp"
)

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

type Asset struct {
	Checksum      string  `json:"checksum"`
	DeviceAssetID string  `json:"deviceAssetId"`
	DeviceID      string  `json:"deviceId"`
	DuplicateID   *string `json:"duplicateId"`
	Duration      string  `json:"duration"`
	ExifInfo      *struct {
		City             *string  `json:"city"`
		Country          *string  `json:"country"`
		DateTimeOriginal *string  `json:"dateTimeOriginal"`
		Description      *string  `json:"description"`
		ExifImageHeight  *int     `json:"exifImageHeight"`
		ExifImageWidth   *int     `json:"exifImageWidth"`
		ExposureTime     *string  `json:"exposureTime"`
		FNumber          *float64 `json:"fNumber"`
		FileSizeInByte   *int     `json:"fileSizeInByte"`
		FocalLength      *float64 `json:"focalLength"`
		Iso              *int     `json:"iso"`
		Latitude         *float64 `json:"latitude"`
		LensModel        *string  `json:"lensModel"`
		Longitude        *float64 `json:"longitude"`
		Make             *string  `json:"make"`
		Model            *string  `json:"model"`
		ModifyDate       *string  `json:"modifyDate"`
		Orientation      *string  `json:"orientation"`
		ProjectionType   *string  `json:"projectionType"`
		Rating           *float64 `json:"rating"`
		State            *string  `json:"state"`
		TimeZone         *string  `json:"timeZone"`
	} `json:"exifInfo"`
	FileCreatedAt    string                   `json:"fileCreatedAt"`
	FileModifiedAt   string                   `json:"fileModifiedAt"`
	HasMetadata      bool                     `json:"hasMetadata"`
	ID               string                   `json:"id"`
	IsArchived       bool                     `json:"isArchived"`
	IsFavorite       bool                     `json:"isFavorite"`
	IsOffline        bool                     `json:"isOffline"`
	IsTrashed        bool                     `json:"isTrashed"`
	LibraryID        *string                  `json:"libraryId"`
	LivePhotoVideoID *string                  `json:"livePhotoVideoId"`
	LocalDateTime    string                   `json:"localDateTime"`
	OriginalFileName string                   `json:"originalFileName"`
	OriginalMimeType *string                  `json:"originalMimeType"`
	OriginalPath     string                   `json:"originalPath"`
	OwnerID          string                   `json:"ownerId"`
	People           []AssetFaceWithoutPerson `json:"people"`
	Resized          *bool                    `json:"resized"`
	Stack            *AssetStack              `json:"stack"`
	// Tags ??
	Thumbhash *string `json:"thumbhash"`
	// UnassignedFaces ??
	Type      string `json:"type"`
	UpdatedAt string `json:"updatedAt"`
}

type AssetFaceWithoutPerson struct {
	BoundingBoxX1 int     `json:"boundingBoxX1"`
	BoundingBoxX2 int     `json:"boundingBoxX2"`
	BoundingBoxY1 int     `json:"boundingBoxY1"`
	BoundingBoxY2 int     `json:"boundingBoxY2"`
	ID            string  `json:"id"`
	ImageHeight   int     `json:"imageHeight"`
	ImageWidth    int     `json:"imageWidth"`
	SourceType    *string `json:"sourceType"`
}

type AssetStack struct {
	AssetCount     int    `json:"assetCount"`
	ID             string `json:"id"`
	PrimaryAssetId int    `json:"primaryAssetId"`
}
