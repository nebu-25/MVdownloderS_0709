package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestMetadataReturnsOnlyPreMuxedFormats(t *testing.T) {
	ytdlp := newFakeService(t, fakeMetadataYTDLP(t), fakeFFprobe(t, true))

	metadata, err := ytdlp.Metadata(context.Background(), "https://x.com/user/status/123")
	if err != nil {
		t.Fatalf("Metadata() error = %v", err)
	}
	if len(metadata.Formats) != 1 {
		t.Fatalf("got %d formats, want 1", len(metadata.Formats))
	}
	if metadata.Formats[0].FormatID != "18" || metadata.Formats[0].NeedsMerge {
		t.Errorf("unexpected muxed format: %+v", metadata.Formats[0])
	}
}

func TestMetadataReturnsDASHFormatsWithProvider(t *testing.T) {
	ytdlp := newFakeServiceWithProvider(
		t,
		fakeMetadataYTDLP(t),
		fakeFFprobe(t, true),
		"http://pot-provider.railway.internal:4416",
		"",
		"",
		"node",
	)

	metadata, err := ytdlp.Metadata(context.Background(), "https://youtube.com/watch?v=test")
	if err != nil {
		t.Fatalf("Metadata() error = %v", err)
	}
	if len(metadata.Formats) != 2 {
		t.Fatalf("got %d formats, want 2", len(metadata.Formats))
	}
	if metadata.Formats[1].FormatID != "137+140" || !metadata.Formats[1].NeedsMerge {
		t.Errorf("unexpected DASH format: %+v", metadata.Formats[1])
	}
	args := strings.Join(ytdlp.runtimeArgs(), " ")
	if !strings.Contains(args, "youtubepot-bgutilhttp:base_url=http://pot-provider.railway.internal:4416") {
		t.Errorf("runtime args do not contain provider URL: %q", args)
	}
}

func TestRuntimeArgsIncludesCookiesPath(t *testing.T) {
	ytdlp := newFakeServiceWithOptions(
		t,
		fakeMetadataYTDLP(t),
		fakeFFprobe(t, true),
		"",
		"",
		"/run/secrets/youtube-cookies.txt",
		"node",
	)

	args := strings.Join(ytdlp.runtimeArgs(), " ")
	if !strings.Contains(args, "--cookies /run/secrets/youtube-cookies.txt") {
		t.Errorf("runtime args do not contain cookies path: %q", args)
	}
}

func TestRuntimeArgsIncludesProxyAndJSRuntime(t *testing.T) {
	ytdlp := newFakeServiceWithOptions(
		t,
		fakeMetadataYTDLP(t),
		fakeFFprobe(t, true),
		"",
		"http://user:pass@proxy.example:8080",
		"",
		"node",
	)

	args := strings.Join(ytdlp.runtimeArgs(), " ")
	if !strings.Contains(args, "--proxy http://user:pass@proxy.example:8080") {
		t.Errorf("runtime args do not contain proxy: %q", args)
	}
	if !strings.Contains(args, "--js-runtimes node") {
		t.Errorf("runtime args do not contain js runtime: %q", args)
	}
}

func TestPrepareDownloadReturnsCompleteVerifiedFile(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(source, []byte("complete-media"), 0o600); err != nil {
		t.Fatal(err)
	}
	ytdlp := newFakeService(t, fakeDownloadYTDLP(t, source), fakeFFprobe(t, true))

	download, err := ytdlp.PrepareDownload(
		context.Background(),
		"https://www.youtube.com/watch?v=test",
		"18",
	)
	if err != nil {
		t.Fatalf("PrepareDownload() error = %v", err)
	}
	tempDir := download.dir
	content, err := os.ReadFile(filepath.Join(tempDir, "media.mp4"))
	if err != nil {
		t.Fatalf("read prepared file: %v", err)
	}
	if string(content) != "complete-media" {
		t.Fatalf("content = %q", content)
	}
	if download.Size() != int64(len(content)) {
		t.Fatalf("size = %d, want %d", download.Size(), len(content))
	}
	if err := download.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("temporary directory still exists after Close(): %v", err)
	}
}

func TestPrepareDownloadRejectsMissingTrack(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(source, []byte("audio-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	ytdlp := newFakeService(t, fakeDownloadYTDLP(t, source), fakeFFprobe(t, false))

	if _, err := ytdlp.PrepareDownload(
		context.Background(),
		"https://www.youtube.com/watch?v=test",
		"18",
	); !errors.Is(err, ErrMediaVerificationFailed) {
		t.Fatalf("error = %v, want ErrMediaVerificationFailed", err)
	}
}

func TestPrepareDownloadWithRealFFprobe(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}

	source := filepath.Join(t.TempDir(), "source.mp4")
	runCommand(t, ffmpegPath,
		"-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=blue:s=160x90:d=0.5",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=0.5",
		"-c:v", "libx264", "-c:a", "aac", "-shortest", source,
	)
	ytdlp := NewYTDLP(
		fakeDownloadYTDLP(t, source),
		ffmpegPath,
		ffprobePath,
		"450M",
		"",
		"",
		"",
		"node",
		5*time.Second,
		zerolog.Nop(),
	)

	download, err := ytdlp.PrepareDownload(
		context.Background(),
		"https://www.youtube.com/watch?v=test",
		"18",
	)
	if err != nil {
		t.Fatalf("PrepareDownload() error = %v", err)
	}
	if download.Size() <= 0 {
		t.Fatal("prepared file is empty")
	}
	if err := download.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func newFakeService(t *testing.T, ytdlpPath, ffprobePath string) *YTDLP {
	t.Helper()
	return newFakeServiceWithProvider(t, ytdlpPath, ffprobePath, "", "", "", "node")
}

func newFakeServiceWithProvider(
	t *testing.T,
	ytdlpPath, ffprobePath, potProviderURL, proxy, cookiesPath, jsRuntime string,
) *YTDLP {
	t.Helper()
	return newFakeServiceWithOptions(
		t,
		ytdlpPath,
		ffprobePath,
		potProviderURL,
		proxy,
		cookiesPath,
		jsRuntime,
	)
}

func newFakeServiceWithOptions(
	t *testing.T,
	ytdlpPath, ffprobePath, potProviderURL, proxy, cookiesPath, jsRuntime string,
) *YTDLP {
	t.Helper()
	return NewYTDLP(
		ytdlpPath,
		"ffmpeg",
		ffprobePath,
		"450M",
		potProviderURL,
		proxy,
		cookiesPath,
		jsRuntime,
		time.Second,
		zerolog.Nop(),
	)
}

func fakeMetadataYTDLP(t *testing.T) string {
	t.Helper()
	script := `#!/bin/sh
case "$*" in
	*"--ignore-config"*) ;;
	*) exit 3 ;;
esac
case "$*" in
	*"--js-runtimes node"*) ;;
	*) exit 3 ;;
esac
printf '%s' '{
	"title":"test","thumbnail":"https://example.com/image.jpg","duration":10,
	"formats":[
		{"format_id":"18","ext":"mp4","resolution":"360p","vcodec":"avc1","acodec":"mp4a"},
		{"format_id":"140","ext":"m4a","abr":129,"vcodec":"none","acodec":"mp4a"},
		{"format_id":"137","ext":"mp4","resolution":"1080p","vcodec":"avc1","acodec":"none"},
		{"format_id":"248","ext":"mp4","resolution":"1080p","vcodec":"vp9","acodec":"none"},
		{"format_id":"bad","ext":"mp4","resolution":"720p","vcodec":"avc1","acodec":"opus"}
	]
}'
`
	return writeExecutable(t, "fake-metadata-yt-dlp", script)
}

func fakeDownloadYTDLP(t *testing.T, source string) string {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
case "$*" in
	*"--ignore-config"*) ;;
	*) exit 3 ;;
esac
case "$*" in
	*"--js-runtimes node"*) ;;
	*) exit 3 ;;
esac
output=""
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-o" ]; then
		shift
		output="$1"
		break
	fi
	shift
done
[ -n "$output" ] || exit 2
cp %q "$output"
printf '%%s\n' "$output"
`, source)
	return writeExecutable(t, "fake-download-yt-dlp", script)
}

func fakeFFprobe(t *testing.T, hasVideo bool) string {
	t.Helper()
	tracks := "audio"
	if hasVideo {
		tracks = "video\\naudio"
	}
	return writeExecutable(t, "fake-ffprobe",
		fmt.Sprintf("#!/bin/sh\nprintf '%%b\\n' %q\n", tracks))
}

func writeExecutable(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", name, err)
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
