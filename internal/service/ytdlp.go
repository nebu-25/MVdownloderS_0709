package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/nebu-25/MVdownloderS_0709/internal/model"
)

var ErrExtractionFailed = errors.New("media extraction failed")

type YTDLP struct {
	binary  string
	timeout time.Duration
	logger  zerolog.Logger
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

func NewYTDLP(binary string, timeout time.Duration, logger zerolog.Logger) *YTDLP {
	return &YTDLP{binary: binary, timeout: timeout, logger: logger}
}

func (y *YTDLP) Metadata(parent context.Context, mediaURL string) (model.MetadataResponse, error) {
	ctx, cancel := context.WithTimeout(parent, y.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, y.binary,
		"--dump-single-json",
		"--no-playlist",
		"--no-warnings",
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
	var audio *rawFormat
	for i := range raw.Formats {
		format := &raw.Formats[i]
		if format.VCodec == "none" && format.ACodec != "none" &&
			(format.Ext == "m4a" || format.Ext == "mp4") {
			if audio == nil || formatBitrate(*format) > formatBitrate(*audio) {
				audio = format
			}
		}
	}

	for _, format := range raw.Formats {
		if format.FormatID == "" || format.Ext != "mp4" || format.VCodec == "none" {
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

		result.Formats = append(result.Formats, model.Format{
			FormatID: format.FormatID, Resolution: resolution, Ext: format.Ext,
			Filesize: filesize, VideoCodec: format.VCodec, AudioCodec: format.ACodec,
		})
	}
	return result, nil
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

func (y *YTDLP) Stream(
	parent context.Context,
	mediaURL, formatID string,
	destination io.Writer,
	onStderr func(string),
) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	args := []string{
		"--no-playlist",
		"--no-warnings",
		"--no-progress",
		"--downloader", "ffmpeg",
		"--merge-output-format", "mp4",
	}
	args = append(args, "-f", formatID, "-o", "-", "--", mediaURL)
	cmd := exec.CommandContext(ctx, y.binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrExtractionFailed, err)
	}
	if err := cmd.Start(); err != nil {
		y.logCommandError("download_start", err)
		return ErrExtractionFailed
	}

	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if onStderr != nil {
				onStderr(scanner.Text())
			}
		}
	}()

	_, copyErr := io.Copy(destination, stdout)
	if copyErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	<-stderrDone

	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		y.logCommandError("download", waitErr)
		return ErrExtractionFailed
	}
	return nil
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
