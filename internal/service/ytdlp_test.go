package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestMetadataBuildsMuxedAndDASHFormats(t *testing.T) {
	ytdlp := NewYTDLP(fakeYTDLP(t), fakeFFmpeg(t), time.Second, zerolog.Nop())

	metadata, err := ytdlp.Metadata(context.Background(), "https://x.com/user/status/123")
	if err != nil {
		t.Fatalf("Metadata() error = %v", err)
	}
	if len(metadata.Formats) != 2 {
		t.Fatalf("got %d formats, want 2", len(metadata.Formats))
	}
	if metadata.Formats[0].FormatID != "18" || metadata.Formats[0].NeedsMerge {
		t.Errorf("unexpected muxed format: %+v", metadata.Formats[0])
	}
	if metadata.Formats[1].FormatID != "137+140" || !metadata.Formats[1].NeedsMerge {
		t.Errorf("unexpected DASH format: %+v", metadata.Formats[1])
	}
}

func TestStreamSingleFormatPassesThroughYTDLP(t *testing.T) {
	ytdlp := NewYTDLP(fakeYTDLP(t), fakeFFmpeg(t), time.Second, zerolog.Nop())
	var output bytes.Buffer
	var stderr []string

	err := ytdlp.Stream(
		context.Background(),
		"https://www.youtube.com/watch?v=test",
		"18",
		&output,
		func(line string) { stderr = append(stderr, line) },
	)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if output.String() != "media" {
		t.Fatalf("output = %q, want media", output.String())
	}
	args := strings.Join(stderr, " ")
	if !strings.Contains(args, "-f 18 -o -") {
		t.Errorf("arguments %q do not contain the selected format", args)
	}
	if strings.Contains(args, "--downloader ffmpeg") {
		t.Errorf("single format unexpectedly uses the ffmpeg downloader: %q", args)
	}
}

func TestStreamMergedUsesSeparatePipes(t *testing.T) {
	ytdlp := NewYTDLP(fakeYTDLP(t), fakeFFmpeg(t), time.Second, zerolog.Nop())
	var output bytes.Buffer
	var stderr []string

	err := ytdlp.Stream(
		context.Background(),
		"https://www.youtube.com/watch?v=test",
		"137+140",
		&output,
		func(line string) { stderr = append(stderr, line) },
	)
	if err != nil {
		t.Fatalf("Stream() error = %v; stderr = %v", err, stderr)
	}
	if output.String() != "video+audio" {
		t.Fatalf("output = %q, want video+audio", output.String())
	}
	logs := strings.Join(stderr, " ")
	for _, expected := range []string{
		"video: ", "-f 137 -o -",
		"audio: ", "-f 140 -o -",
		"ffmpeg: ", "-map 0:v:0", "-map 1:a:0",
		"frag_keyframe+empty_moov+default_base_moof",
	} {
		if !strings.Contains(logs, expected) {
			t.Errorf("process logs %q do not contain %q", logs, expected)
		}
	}
}

func TestStreamMergedProducesVideoAndAudioTracks(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "video.mp4")
	audioPath := filepath.Join(dir, "audio.m4a")
	runCommand(t, ffmpegPath,
		"-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=blue:s=160x90:d=0.5",
		"-an", "-c:v", "libx264", "-movflags", "+faststart", videoPath,
	)
	runCommand(t, ffmpegPath,
		"-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=0.5",
		"-vn", "-c:a", "aac", "-movflags", "+faststart", audioPath,
	)

	ytdlp := NewYTDLP(
		fakeMediaYTDLP(t, videoPath, audioPath),
		ffmpegPath,
		5*time.Second,
		zerolog.Nop(),
	)
	var output bytes.Buffer
	if err := ytdlp.Stream(
		context.Background(),
		"https://www.youtube.com/watch?v=test",
		"137+140",
		&output,
		nil,
	); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	mergedPath := filepath.Join(dir, "merged.mp4")
	if err := os.WriteFile(mergedPath, output.Bytes(), 0o600); err != nil {
		t.Fatalf("write merged output: %v", err)
	}
	probe := runCommand(t, ffprobePath,
		"-v", "error",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		mergedPath,
	)
	tracks := string(probe)
	if !strings.Contains(tracks, "video") || !strings.Contains(tracks, "audio") {
		t.Fatalf("merged tracks = %q, want video and audio", tracks)
	}
}

func fakeYTDLP(t *testing.T) string {
	t.Helper()
	script := `#!/bin/sh
case "$*" in
	*"--dump-single-json"*)
		printf '%s' '{
			"title":"test","thumbnail":"https://example.com/image.jpg","duration":10,
			"formats":[
				{"format_id":"18","ext":"mp4","resolution":"360p","vcodec":"avc1","acodec":"mp4a"},
				{"format_id":"140","ext":"m4a","abr":129,"vcodec":"none","acodec":"mp4a"},
				{"format_id":"139","ext":"m4a","abr":49,"vcodec":"none","acodec":"mp4a"},
				{"format_id":"137","ext":"mp4","resolution":"1080p","vcodec":"avc1","acodec":"none"},
				{"format_id":"248","ext":"mp4","resolution":"1080p","vcodec":"vp9","acodec":"none"},
				{"format_id":"bad","ext":"mp4","resolution":"720p","vcodec":"avc1","acodec":"opus"}
			]
		}'
		;;
	*"-f 137 "*)
		printf '%s' 'video'
		printf '%s\n' "$*" >&2
		;;
	*"-f 140 "*)
		printf '%s' 'audio'
		printf '%s\n' "$*" >&2
		;;
	*)
		printf '%s' 'media'
		printf '%s\n' "$*" >&2
		;;
esac
`
	path := filepath.Join(t.TempDir(), "fake-yt-dlp")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake yt-dlp: %v", err)
	}
	return path
}

func fakeFFmpeg(t *testing.T) string {
	t.Helper()
	script := `#!/bin/sh
video=$(cat <&3)
audio=$(cat <&4)
printf '%s+%s' "$video" "$audio"
printf '%s\n' "$*" >&2
`
	path := filepath.Join(t.TempDir(), "fake-ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return path
}

func fakeMediaYTDLP(t *testing.T, videoPath, audioPath string) string {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
case "$*" in
	*"-f 137 "*) cat %q ;;
	*"-f 140 "*) cat %q ;;
	*) exit 2 ;;
esac
`, videoPath, audioPath)
	path := filepath.Join(t.TempDir(), "fake-media-yt-dlp")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write media yt-dlp: %v", err)
	}
	return path
}

func runCommand(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", name, err, output)
	}
	return output
}
