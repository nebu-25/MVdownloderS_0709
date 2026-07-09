package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/nebu-25/MVdownloderS_0709/internal/model"
)

func TestDownloadJobManagerTransitionsToReady(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(source, []byte("complete-media"), 0o600); err != nil {
		t.Fatal(err)
	}
	ytdlp := newFakeService(t, fakeDownloadYTDLP(t, source), fakeFFprobe(t, true))
	jobs := NewDownloadJobManager(ytdlp, zerolog.Nop())

	started := jobs.Start("https://www.youtube.com/watch?v=test", "18")
	if started.Status != DownloadJobQueued {
		t.Fatalf("status = %q, want queued", started.Status)
	}

	deadline := time.Now().Add(2 * time.Second)
	var current model.DownloadJobResponse
	var ok bool
	for time.Now().Before(deadline) {
		current, ok = jobs.Get(started.JobID)
		if !ok {
			t.Fatal("job disappeared unexpectedly")
		}
		if current.Status == DownloadJobReady {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if current.Status != DownloadJobReady {
		t.Fatalf("status = %q, want ready", current.Status)
	}

	download, current, ok := jobs.ConsumeDownload(started.JobID)
	if !ok || download == nil {
		t.Fatalf("ConsumeDownload() failed: ok=%v status=%+v", ok, current)
	}
	if current.DownloadURL == "" {
		t.Fatal("expected download url in ready job response")
	}
	if err := download.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestErrorDetailFromClassifications(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{
			name: "provider",
			err:  ErrProviderUnavailable,
			code: "POT_PROVIDER_UNREACHABLE",
		},
		{
			name: "blocked",
			err:  ErrSourceBlocked,
			code: "YTDLP_403",
		},
		{
			name: "missing output",
			err:  ErrOutputNotCreated,
			code: "OUTPUT_NOT_CREATED",
		},
		{
			name: "ffprobe",
			err:  ErrMediaVerificationFailed,
			code: "FFPROBE_FAILED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detail := errorDetailFrom(tc.err)
			if detail == nil || detail.Code != tc.code {
				t.Fatalf("detail = %+v, want code %q", detail, tc.code)
			}
		})
	}
}
