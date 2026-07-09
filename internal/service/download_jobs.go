package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/nebu-25/MVdownloderS_0709/internal/model"
)

const (
	DownloadJobQueued  = "queued"
	DownloadJobRunning = "running"
	DownloadJobReady   = "ready"
	DownloadJobFailed  = "failed"
)

type DownloadJob struct {
	ID       string
	URL      string
	FormatID string
	Status   string
	Created  time.Time
	Updated  time.Time
	Download *PreparedDownload
	Err      *model.ErrorDetail
}

type DownloadJobManager struct {
	ytdlp  *YTDLP
	logger zerolog.Logger

	mu   sync.RWMutex
	jobs map[string]*DownloadJob
}

func NewDownloadJobManager(ytdlp *YTDLP, logger zerolog.Logger) *DownloadJobManager {
	return &DownloadJobManager{
		ytdlp:  ytdlp,
		logger: logger,
		jobs:   make(map[string]*DownloadJob),
	}
}

func (m *DownloadJobManager) Start(url, formatID string) model.DownloadJobResponse {
	now := time.Now().UTC()
	job := &DownloadJob{
		ID:       newDownloadJobID(),
		URL:      url,
		FormatID: formatID,
		Status:   DownloadJobQueued,
		Created:  now,
		Updated:  now,
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	go m.run(job.ID)
	return m.snapshot(job)
}

func (m *DownloadJobManager) Get(jobID string) (model.DownloadJobResponse, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return model.DownloadJobResponse{}, false
	}
	return m.snapshot(job), true
}

func (m *DownloadJobManager) ConsumeDownload(jobID string) (*PreparedDownload, model.DownloadJobResponse, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return nil, model.DownloadJobResponse{}, false
	}
	if job.Status != DownloadJobReady || job.Download == nil {
		return nil, m.snapshot(job), true
	}
	download := job.Download
	delete(m.jobs, jobID)
	return download, m.snapshot(job), true
}

func (m *DownloadJobManager) run(jobID string) {
	m.update(jobID, func(job *DownloadJob) {
		job.Status = DownloadJobRunning
	})

	job := m.get(jobID)
	if job == nil {
		return
	}

	download, err := m.ytdlp.PrepareDownload(context.Background(), job.URL, job.FormatID)
	if err != nil {
		m.update(jobID, func(job *DownloadJob) {
			job.Status = DownloadJobFailed
			job.Err = errorDetailFrom(err)
		})
		return
	}

	m.update(jobID, func(job *DownloadJob) {
		job.Status = DownloadJobReady
		job.Download = download
		job.Err = nil
	})
}

func (m *DownloadJobManager) get(jobID string) *DownloadJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs[jobID]
}

func (m *DownloadJobManager) update(jobID string, fn func(job *DownloadJob)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return
	}
	fn(job)
	job.Updated = time.Now().UTC()
}

func (m *DownloadJobManager) snapshot(job *DownloadJob) model.DownloadJobResponse {
	resp := model.DownloadJobResponse{
		JobID:     job.ID,
		Status:    job.Status,
		URL:       job.URL,
		FormatID:  job.FormatID,
		CreatedAt: job.Created,
		UpdatedAt: job.Updated,
	}
	if job.Status == DownloadJobReady {
		resp.DownloadURL = "/api/v1/download-jobs/" + job.ID + "/download"
	}
	if job.Err != nil {
		resp.Error = job.Err
	}
	return resp
}

func newDownloadJobID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(buf)
}

func errorDetailFrom(err error) *model.ErrorDetail {
	detail := model.ErrorDetail{
		Code:    "EXTRACTION_FAILED",
		Message: "영상 정보를 가져오지 못했습니다.",
	}
	switch {
	case errors.Is(err, ErrProviderUnavailable):
		detail.Code = "POT_PROVIDER_UNREACHABLE"
		detail.Message = "PO Token Provider에 연결할 수 없습니다."
	case errors.Is(err, ErrSourceBlocked):
		detail.Code = "YTDLP_403"
		detail.Message = "영상 소스가 다운로드를 거부했습니다."
	case errors.Is(err, ErrOutputNotCreated):
		detail.Code = "OUTPUT_NOT_CREATED"
		detail.Message = "다운로드 파일이 생성되지 않았습니다."
	case errors.Is(err, ErrMediaVerificationFailed):
		detail.Code = "FFPROBE_FAILED"
		detail.Message = "생성된 파일 검증에 실패했습니다."
	case errors.Is(err, ErrExtractionFailed):
		detail.Message = "영상 정보를 가져오지 못했습니다."
	}
	return &detail
}
