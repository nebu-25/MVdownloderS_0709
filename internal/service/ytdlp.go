package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	binary         string
	ffmpegPath     string
	ffprobePath    string
	maxFileSize    string
	potProviderURL string
	timeout        time.Duration
	logger         zerolog.Logger
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
	binary, ffmpegPath, ffprobePath, maxFileSize, potProviderURL string,
	timeout time.Duration,
	logger zerolog.Logger,
) *YTDLP {
	return &YTDLP{
		binary: binary, ffmpegPath: ffmpegPath, ffprobePath: ffprobePath,
		maxFileSize: maxFileSize, potProviderURL: strings.TrimRight(potProviderURL, "/"),
		timeout: timeout, logger: logger,
	}
}

func (y *YTDLP) SupportsDASH() bool {
	return y.potProviderURL != ""
}

func (y *YTDLP) Metadata(parent context.Context, mediaURL string) (model.MetadataResponse, error) {
	ctx, cancel := context.WithTimeout(parent, y.timeout)
	defer cancel()

	args := []string{
		"--dump-single-json",
		"--no-playlist",
		"--no-warnings",
	}
	args = append(args, y.runtimeArgs()...)
	args = append(args, "--", mediaURL)
	cmd := exec.CommandContext(ctx, y.binary, args...)
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
	var audio *rawFormat
	if y.SupportsDASH() {
		for i := range raw.Formats {
			format := &raw.Formats[i]
			if format.VCodec == "none" && isMP4AudioCodec(format.ACodec) &&
				(format.Ext == "m4a" || format.Ext == "mp4") {
				if audio == nil || formatBitrate(*format) > formatBitrate(*audio) {
					audio = format
				}
			}
		}
	}
	for _, format := range raw.Formats {
		if format.FormatID == "" || format.Ext != "mp4" ||
			!isMP4VideoCodec(format.VCodec) {
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

		if format.ACodec == "none" {
			if audio == nil {
				continue
			}
			result.Formats = append(result.Formats, model.Format{
				FormatID:   format.FormatID + "+" + audio.FormatID,
				Resolution: resolution,
				Ext:        "mp4",
				Filesize:   addFilesizes(filesize, effectiveFilesize(*audio)),
				VideoCodec: format.VCodec,
				AudioCodec: audio.ACodec,
				NeedsMerge: true,
			})
			continue
		}
		if !isMP4AudioCodec(format.ACodec) {
			continue
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

func addFilesizes(left, right *int64) *int64 {
	if left == nil || right == nil {
		return nil
	}
	total := *left + *right
	return &total
}

func formatBitrate(format rawFormat) int64 {
	if format.AudioBitrate > 0 {
		return int64(format.AudioBitrate * 1000)
	}
	if size := effectiveFilesize(format); size != nil {
		return *size
	}
	return 0
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
		"--force-overwrites",
		"--ffmpeg-location", y.ffmpegPath,
		"--merge-output-format", "mp4",
		"--remux-video", "mp4",
		"--max-filesize", y.maxFileSize,
		"--print", "after_move:filepath",
		"-f", formatID,
		"-o", outputPath,
	}
	args = append(args, y.runtimeArgs()...)
	args = append(args, "--", mediaURL)
	cmd := exec.CommandContext(parent, y.binary, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: prepare stdout pipe: %v", ErrExtractionFailed, err)
	}
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: start download: %v", ErrExtractionFailed, err)
	}
	outputBytes, readErr := io.ReadAll(stdout)
	waitErr := cmd.Wait()
	if readErr != nil {
		cleanup()
		return nil, fmt.Errorf("%w: read download output: %v", ErrExtractionFailed, readErr)
	}
	if waitErr != nil {
		cleanup()
		y.logOutputError("download_prepare", waitErr, []byte(stderr.String()))
		return nil, ErrExtractionFailed
	}

	actualPath := strings.TrimSpace(string(outputBytes))
	if actualPath == "" {
		actualPath = outputPath
	}
	if !filepath.IsAbs(actualPath) {
		actualPath = filepath.Join(tempDir, actualPath)
	}
	resolvedPath, err := y.ensureDownloadPath(actualPath, outputPath)
	if err != nil {
		cleanup()
		return nil, err
	}

	if err := y.verifyMedia(parent, resolvedPath); err != nil {
		cleanup()
		return nil, ErrExtractionFailed
	}

	file, err := os.Open(resolvedPath)
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

func (y *YTDLP) ensureDownloadPath(actualPath, fallbackPath string) (string, error) {
	if _, err := os.Stat(actualPath); err == nil {
		return actualPath, nil
	}
	if actualPath != fallbackPath {
		if _, err := os.Stat(fallbackPath); err == nil {
			return fallbackPath, nil
		}
	}

	dir := filepath.Dir(fallbackPath)
	matches, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return "", fmt.Errorf("%w: inspect download directory: %v", ErrExtractionFailed, err)
	}
	if len(matches) == 1 {
		if _, err := os.Stat(matches[0]); err == nil {
			return matches[0], nil
		}
	}

	if len(matches) > 0 {
		return "", fmt.Errorf("%w: output file not created: %s (files: %s)", ErrExtractionFailed, actualPath, strings.Join(matches, ", "))
	}
	return "", fmt.Errorf("%w: output file not created: %s", ErrExtractionFailed, actualPath)
}

func (y *YTDLP) runtimeArgs() []string {
	args := []string{"--js-runtimes", "deno"}
	if y.SupportsDASH() {
		args = append(args,
			"--extractor-args",
			"youtubepot-bgutilhttp:base_url="+y.potProviderURL,
		)
	}
	return args
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
	// Log full stderr for debugging, split into chunks if too large
	if len(message) > 5000 {
		// Log first 2500 chars
		y.logger.Error().
			Str("operation", operation).
			Str("stderr_start", message[:2500]).
			Err(err).
			Msg("media command failed (stderr start)")
		// Log last 2500 chars
		y.logger.Error().
			Str("operation", operation).
			Str("stderr_end", message[len(message)-2500:]).
			Err(err).
			Msg("media command failed (stderr end)")
	} else {
		y.logger.Error().
			Str("operation", operation).
			Str("stderr", message).
			Err(err).
			Msg("media command failed")
	}
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

