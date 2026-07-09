# Video Downloader MVP

YouTube, X(구 Twitter), TikTok의 공개 영상 메타데이터를 조회하고, 선택한
포맷을 디스크 저장 없이 HTTP 응답으로 전달하는 Go/Fiber API입니다. 모든
다운로드는 ffmpeg downloader를 기본으로 사용하며, DASH 비디오·오디오
분리 포맷도 stdout에서 MP4로 병합합니다.

본인 콘텐츠 백업 또는 권한을 가진 콘텐츠에만 사용해야 합니다. DRM 우회는
지원하지 않습니다.

## 실행

필수 도구:

- Go 1.22 이상
- `yt-dlp`
- `ffmpeg` (DASH 비디오·오디오 병합용)
- Node.js 등 yt-dlp가 지원하는 JavaScript 런타임

```bash
go mod download
go run ./cmd/server
```

Docker 실행:

```bash
docker build -t video-downloader .
docker run --rm -p 8080:8080 video-downloader
```

## API

상태 확인:

```bash
curl http://localhost:8080/api/v1/health
```

메타데이터 조회:

```bash
curl -X POST http://localhost:8080/api/v1/metadata \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://www.youtube.com/watch?v=VIDEO_ID"}'
```

응답의 `formats`에 포함된 `format_id`로 다운로드합니다. `needs_merge`가
`true`이면 `137+140`처럼 비디오와 오디오를 결합하는 포맷입니다.

```bash
curl -G http://localhost:8080/api/v1/download \
  --data-urlencode 'url=https://www.youtube.com/watch?v=VIDEO_ID' \
  --data-urlencode 'format_id=FORMAT_ID' \
  -o video.mp4
```

서버는 메타데이터에 포함된 MP4 결합 포맷과 DASH 조합을 반환합니다.
두 개를 초과하는 포맷 조합이나 임의의 yt-dlp 선택 표현식은 허용하지 않습니다.

## 환경 변수

| 이름 | 기본값 | 설명 |
|---|---:|---|
| `PORT` | `8080` | HTTP 포트 |
| `RATE_LIMIT_PER_IP` | `2` | IP별 동시 다운로드 요청 제한 |
| `YTDLP_PATH` | `yt-dlp` | yt-dlp 실행 파일 경로 |
| `YTDLP_TIMEOUT` | `30s` | 메타데이터 추출 제한 시간 |

## 검증

```bash
go test ./...
go vet ./...
```
