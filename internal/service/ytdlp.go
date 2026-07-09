package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/nebu-25/MVdownloderS_0709/internal/model"
)

var ErrExtractionFailed = errors.New("media extraction failed")

type YTDLP struct {
	binary      string
	ffmpegPath  string
	ffprobePath string
	maxFileSize string
	timeout     time.Duration
	logger      zerolog.Logger
}

type rawMetadata struct {
	Title     string      `json:"title"`
	Thumbnail string      `json:"thumbnail"`
	Duration  float64     `json:"duration"`
	Formats   []rawFormat `json:"formats"`
}

type rawFormat struct {
	FormatID       string  `json:"format_id"`
	Ext            string  `json:"ext"`
	Resolution     string  `json:"resolution"`
	FormatNote     string  `json:"format_note"`
	Filesize       *int64  `json:"filesize"`
	FilesizeApprox *int64  `json:"filesize_approx"`
	AudioBitrate   float64 `json:"abr"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
}

func NewYTDLP(
	binary, ffmpegPath, ffprobePath, maxFileSize string,
	timeout time.Duration,
	logger zerolog.Logger,
) *YTDLP {
	return &YTDLP{
		binary: binary, ffmpegPath: ffmpegPath, ffprobePath: ffprobePath,
		maxFileSize: maxFileSize, timeout: timeout, logger: logger,
	}
}

func (y *YTDLP) Metadata(parent context.Context, mediaURL string) (model.MetadataResponse, error) {
	ctx, cancel := context.WithTimeout(parent, y.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, y.binary,
		"--dump-single-json",
		"--no-playlist",
		"--no-warnings",
		"--js-runtimes", "deno",
		"--",
		mediaURL,
	)
	output, err := cmd.Output()
	if err != nil {
		y.logCommandError("metadata", err)
		if ctx.Err() != nil {
			return model.MetadataResponse{}, fmt.Errorf("%w: timeout", ErrExtractionFailed)
		}
		return model.MetadataResponse{}, ErrExtractionFailed
	}

	var raw rawMetadata
	if err := json.Unmarshal(output, &raw); err != nil {
		y.logger.Error().Err(err).Msg("invalid yt-dlp metadata response")
		return model.MetadataResponse{}, ErrExtractionFailed
	}

	result := model.MetadataResponse{
		Title: raw.Title, Thumbnail: raw.Thumbnail, Duration: raw.Duration,
		Formats: make([]model.Format, 0, len(raw.Formats)),
	}
	for _, format := range raw.Formats {
		if format.FormatID == "" || format.Ext != "mp4" ||
			!isMP4VideoCodec(format.VCodec) ||
			!isMP4AudioCodec(format.ACodec) {
			continue
		}
		resolution := format.Resolution
		if resolution == "" {
			resolution = format.FormatNote
		}
		filesize := format.Filesize
		if filesize == nil {
			filesize = format.FilesizeApprox
		}

		result.Formats = append(result.Formats, model.Format{
			FormatID: format.FormatID, Resolution: resolution, Ext: format.Ext,
			Filesize: filesize, VideoCodec: format.VCodec, AudioCodec: format.ACodec,
		})
	}
	return result, nil
}

func isMP4VideoCodec(codec string) bool {
	codec = strings.ToLower(codec)
	return strings.HasPrefix(codec, "avc1") ||
		strings.HasPrefix(codec, "avc") ||
		strings.HasPrefix(codec, "h264")
}

func isMP4AudioCodec(codec string) bool {
	codec = strings.ToLower(codec)
	return strings.HasPrefix(codec, "mp4a") || strings.HasPrefix(codec, "aac")
}

func effectiveFilesize(format rawFormat) *int64 {
	if format.Filesize != nil {
		return format.Filesize
	}
	return format.FilesizeApprox
}

type PreparedDownload struct {
	file *os.File
	dir  string
	size int64
}

func (d *PreparedDownload) Read(buffer []byte) (int, error) {
	return d.file.Read(buffer)
}

func (d *PreparedDownload) Close() error {
	closeErr := d.file.Close()
	removeErr := os.RemoveAll(d.dir)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func (d *PreparedDownload) Size() int64 {
	return d.size
}

func (y *YTDLP) PrepareDownload(
	parent context.Context,
	mediaURL, formatID string,
) (*PreparedDownload, error) {
	tempDir, err := os.MkdirTemp("", "video-download-*")
	if err != nil {
		return nil, fmt.Errorf("%w: create temporary directory: %v", ErrExtractionFailed, err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	outputPath := filepath.Join(tempDir, "media.mp4")
	args := []string{
		"--no-playlist",
		"--no-warnings",
		"--no-progress",
		"--js-runtimes", "deno",
		"--force-overwrites",
		"--ffmpeg-location", y.ffmpegPath,
		"--merge-output-format", "mp4",
		"--remux-video", "mp4",
		"--max-filesize", y.maxFileSize,
		"-f", formatID,
		"-o", outputPath,
		"--",
		mediaURL,
	}
	cmd := exec.CommandContext(parent, y.binary, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		y.logOutputError("download_prepare", err, output)
		return nil, ErrExtractionFailed
	}

	if err := y.verifyMedia(parent, outputPath); err != nil {
		cleanup()
		return nil, ErrExtractionFailed
	}

	file, err := os.Open(outputPath)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: open output: %v", ErrExtractionFailed, err)
	}
	info, err := file.Stat()
	if err != nil || info.Size() <= 0 {
		_ = file.Close()
		cleanup()
		return nil, fmt.Errorf("%w: invalid output file", ErrExtractionFailed)
	}
	return &PreparedDownload{file: file, dir: tempDir, size: info.Size()}, nil
}

func (y *YTDLP) verifyMedia(ctx context.Context, mediaPath string) error {
	cmd := exec.CommandContext(ctx, y.ffprobePath,
		"-v", "error",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		mediaPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		y.logOutputError("ffprobe", err, output)
		return err
	}
	tracks := string(output)
	if !strings.Contains(tracks, "video") || !strings.Contains(tracks, "audio") {
		y.logger.Error().Str("tracks", strings.TrimSpace(tracks)).
			Msg("prepared file is missing video or audio")
		return errors.New("prepared file is missing required media tracks")
	}
	return nil
}

func (y *YTDLP) logOutputError(operation string, err error, output []byte) {
	message := strings.TrimSpace(string(output))
	if len(message) > 2000 {
		message = message[len(message)-2000:]
	}
	y.logger.Error().
		Str("operation", operation).
		Str("output", message).
		Err(err).
		Msg("media command failed")
}

func (y *YTDLP) logCommandError(operation string, err error) {
	event := y.logger.Error().Str("operation", operation)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		message := strings.TrimSpace(string(exitErr.Stderr))
		if len(message) > 1000 {
			message = message[:1000]
		}
		event = event.Str("stderr", message)
	}
	event.Err(err).Msg("yt-dlp command failed")
}
