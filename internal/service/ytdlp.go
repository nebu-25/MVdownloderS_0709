package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/nebu-25/MVdownloderS_0709/internal/model"
)

var ErrExtractionFailed = errors.New("media extraction failed")

type YTDLP struct {
	binary     string
	ffmpegPath string
	timeout    time.Duration
	logger     zerolog.Logger
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

func NewYTDLP(binary, ffmpegPath string, timeout time.Duration, logger zerolog.Logger) *YTDLP {
	return &YTDLP{
		binary: binary, ffmpegPath: ffmpegPath, timeout: timeout, logger: logger,
	}
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
		if format.VCodec == "none" && isMP4AudioCodec(format.ACodec) &&
			(format.Ext == "m4a" || format.Ext == "mp4") {
			if audio == nil || formatBitrate(*format) > formatBitrate(*audio) {
				audio = format
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

func (y *YTDLP) Stream(
	parent context.Context,
	mediaURL, formatID string,
	destination io.Writer,
	onStderr func(string),
) error {
	parts := strings.Split(formatID, "+")
	if len(parts) == 2 {
		return y.streamMerged(parent, mediaURL, parts[0], parts[1], destination, onStderr)
	}
	return y.streamSingle(parent, mediaURL, formatID, destination, onStderr)
}

func (y *YTDLP) streamSingle(
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

func (y *YTDLP) streamMerged(
	parent context.Context,
	mediaURL, videoFormat, audioFormat string,
	destination io.Writer,
	onStderr func(string),
) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	videoReader, videoWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("%w: create video pipe: %v", ErrExtractionFailed, err)
	}
	defer videoReader.Close()
	defer videoWriter.Close()

	audioReader, audioWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("%w: create audio pipe: %v", ErrExtractionFailed, err)
	}
	defer audioReader.Close()
	defer audioWriter.Close()

	videoCmd := y.downloadCommand(ctx, mediaURL, videoFormat)
	videoCmd.Stdout = videoWriter
	audioCmd := y.downloadCommand(ctx, mediaURL, audioFormat)
	audioCmd.Stdout = audioWriter

	ffmpegCmd := exec.CommandContext(ctx, y.ffmpegPath,
		"-hide_banner",
		"-loglevel", "warning",
		"-i", "pipe:3",
		"-i", "pipe:4",
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-c:v", "copy",
		"-c:a", "copy",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-f", "mp4",
		"pipe:1",
	)
	ffmpegCmd.ExtraFiles = []*os.File{videoReader, audioReader}
	ffmpegCmd.Stdout = destination

	videoStderr, err := videoCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("%w: video stderr: %v", ErrExtractionFailed, err)
	}
	audioStderr, err := audioCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("%w: audio stderr: %v", ErrExtractionFailed, err)
	}
	ffmpegStderr, err := ffmpegCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("%w: ffmpeg stderr: %v", ErrExtractionFailed, err)
	}

	stderrDone := make(chan struct{}, 3)
	var stderrMu sync.Mutex
	scanStderr(videoStderr, "video", onStderr, &stderrMu, stderrDone)
	scanStderr(audioStderr, "audio", onStderr, &stderrMu, stderrDone)
	scanStderr(ffmpegStderr, "ffmpeg", onStderr, &stderrMu, stderrDone)

	if err := ffmpegCmd.Start(); err != nil {
		y.logCommandError("ffmpeg_start", err)
		return ErrExtractionFailed
	}
	if err := videoCmd.Start(); err != nil {
		cancel()
		_ = ffmpegCmd.Wait()
		y.logCommandError("video_download_start", err)
		return ErrExtractionFailed
	}
	_ = videoWriter.Close()
	if err := audioCmd.Start(); err != nil {
		cancel()
		_ = videoCmd.Wait()
		_ = ffmpegCmd.Wait()
		y.logCommandError("audio_download_start", err)
		return ErrExtractionFailed
	}
	_ = audioWriter.Close()

	type processResult struct {
		name string
		err  error
	}
	results := make(chan processResult, 3)
	go func() { results <- processResult{"video_download", videoCmd.Wait()} }()
	go func() { results <- processResult{"audio_download", audioCmd.Wait()} }()
	go func() { results <- processResult{"ffmpeg_mux", ffmpegCmd.Wait()} }()

	var firstErr error
	for range 3 {
		result := <-results
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			cancel()
			y.logCommandError(result.name, result.err)
		}
	}
	for range 3 {
		<-stderrDone
	}
	if firstErr != nil {
		return ErrExtractionFailed
	}
	return nil
}

func (y *YTDLP) downloadCommand(ctx context.Context, mediaURL, formatID string) *exec.Cmd {
	return exec.CommandContext(ctx, y.binary,
		"--no-playlist",
		"--no-warnings",
		"--no-progress",
		"-f", formatID,
		"-o", "-",
		"--",
		mediaURL,
	)
}

func scanStderr(
	reader io.Reader,
	prefix string,
	onStderr func(string),
	callbackMu *sync.Mutex,
	done chan<- struct{},
) {
	go func() {
		defer func() { done <- struct{}{} }()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			if onStderr != nil {
				callbackMu.Lock()
				onStderr(prefix + ": " + scanner.Text())
				callbackMu.Unlock()
			}
		}
	}()
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
