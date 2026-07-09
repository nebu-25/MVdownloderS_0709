package model

import "time"

type MetadataRequest struct {
	URL string `json:"url"`
}

type Format struct {
	FormatID   string `json:"format_id"`
	Resolution string `json:"resolution"`
	Ext        string `json:"ext"`
	Filesize   *int64 `json:"filesize,omitempty"`
	VideoCodec string `json:"video_codec,omitempty"`
	AudioCodec string `json:"audio_codec,omitempty"`
	NeedsMerge bool   `json:"needs_merge"`
}

type MetadataResponse struct {
	Title     string   `json:"title"`
	Thumbnail string   `json:"thumbnail"`
	Duration  float64  `json:"duration"`
	Formats   []Format `json:"formats"`
}

type DownloadJobRequest struct {
	URL      string `json:"url"`
	FormatID string `json:"format_id"`
}

type DownloadJobResponse struct {
	JobID       string       `json:"job_id"`
	Status      string       `json:"status"`
	URL         string       `json:"url"`
	FormatID    string       `json:"format_id"`
	DownloadURL string       `json:"download_url,omitempty"`
	Error       *ErrorDetail `json:"error,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
