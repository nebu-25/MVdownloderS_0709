package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestMetadataBuildsMuxedAndDASHFormats(t *testing.T) {
	ytdlp := NewYTDLP(fakeYTDLP(t), time.Second, zerolog.Nop())

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

func TestStreamUsesFFmpegDownloaderByDefault(t *testing.T) {
	for _, formatID := range []string{"18", "137+140"} {
		t.Run(formatID, func(t *testing.T) {
			ytdlp := NewYTDLP(fakeYTDLP(t), time.Second, zerolog.Nop())
			var output bytes.Buffer
			var stderr []string

			err := ytdlp.Stream(
				context.Background(),
				"https://www.youtube.com/watch?v=test",
				formatID,
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
			for _, expected := range []string{
				"--downloader ffmpeg",
				"--merge-output-format mp4",
				"-f " + formatID,
				"-o -",
			} {
				if !strings.Contains(args, expected) {
					t.Errorf("arguments %q do not contain %q", args, expected)
				}
			}
		})
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
				{"format_id":"137","ext":"mp4","resolution":"1080p","vcodec":"avc1","acodec":"none"}
			]
		}'
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
