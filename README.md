# Video Downloader MVP

YouTube, X(구 Twitter), TikTok의 공개 영상 메타데이터를 조회하고, 선택한
포맷을 HTTP 응답으로 전달하는 Go/Fiber API입니다. yt-dlp와 ffmpeg가 임시
MP4를 완성한 후 ffprobe로 영상·음성 트랙을 검증하고 브라우저에 전달합니다.
임시 파일은 응답이 끝나면 즉시 삭제됩니다.

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

브라우저에서 `http://localhost:8080`을 열면 URL 입력, 포맷 선택 및 다운로드가
가능한 웹 화면을 사용할 수 있습니다.

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

응답의 `formats`에 포함된 `format_id`로 다운로드합니다. 기본 상태에서는
영상과 음성이 이미 결합된 H.264/AAC MP4만 반환합니다. PO Token Provider가
설정되면 `137+140` 같은 고화질 DASH 조합도 반환합니다.

```bash
curl -G http://localhost:8080/api/v1/download \
  --data-urlencode 'url=https://www.youtube.com/watch?v=VIDEO_ID' \
  --data-urlencode 'format_id=FORMAT_ID' \
  -o video.mp4
```

임의의 yt-dlp 선택 표현식은 허용하지 않습니다. DASH 조합은
`POT_PROVIDER_URL`이 설정된 경우에만 허용됩니다.

비동기 작업도 사용할 수 있습니다.

```bash
curl -X POST http://localhost:8080/api/v1/download-jobs \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://www.youtube.com/watch?v=VIDEO_ID","format_id":"18"}'
```

응답의 `job_id`로 상태와 다운로드 가능 여부를 조회합니다.

```bash
curl http://localhost:8080/api/v1/download-jobs/JOB_ID
curl -G http://localhost:8080/api/v1/download-jobs/JOB_ID/download
```

## Railway 고화질 YouTube 설정

1. 같은 Railway 프로젝트에 Docker Image 서비스를 추가합니다.
   - 이미지: `brainicism/bgutil-ytdlp-pot-provider:1.3.1-deno`
   - 서비스 이름 예시: `pot-provider`
2. Provider 서비스는 공개 도메인을 만들지 않고 Private Networking만 사용합니다.
3. API 서비스 변수에 다음 값을 추가하고 재배포합니다.

```text
POT_PROVIDER_URL=http://pot-provider.railway.internal:4416
```

Provider가 설정되지 않으면 서비스는 검증된 단일 MP4 포맷으로 자동
폴백합니다. PO Token은 YouTube의 정책 변경이나 IP 상태에 따라 403 차단을
완전히 방지하지 못할 수 있습니다.

## 에러 코드

- `INVALID_URL`
- `INVALID_FORMAT`
- `POT_PROVIDER_UNREACHABLE`
- `YTDLP_403`
- `OUTPUT_NOT_CREATED`
- `FFPROBE_FAILED`
- `EXTRACTION_FAILED`

## 환경 변수

| 이름 | 기본값 | 설명 |
|---|---:|---|
| `PORT` | `8080` | HTTP 포트 |
| `RATE_LIMIT_PER_IP` | `2` | IP별 동시 다운로드 요청 제한 |
| `YTDLP_PATH` | `yt-dlp` | yt-dlp 실행 파일 경로 |
| `FFMPEG_PATH` | `ffmpeg` | ffmpeg 실행 파일 경로 |
| `FFPROBE_PATH` | `ffprobe` | ffprobe 실행 파일 경로 |
| `MAX_DOWNLOAD_SIZE` | `450M` | yt-dlp 입력 포맷별 최대 크기 |
| `POT_PROVIDER_URL` | 미설정 | bgutil PO Token Provider 내부 URL |
| `YTDLP_PROXY` | 미설정 | yt-dlp 요청에 사용할 HTTP/SOCKS 프록시 URL |
| `YTDLP_JS_RUNTIME` | `node` | yt-dlp JavaScript 런타임. 예: `node`, `deno` |
| `YTDLP_COOKIES` | 미설정 | Netscape cookies.txt 파일 내용. 설정하면 `/tmp/youtube-cookies.txt`로 저장해 사용 |
| `YTDLP_COOKIES_PATH` | 미설정 | YouTube 로그인 확인 우회가 필요한 경우 Netscape cookies.txt 파일 경로 |
| `YTDLP_COOKIES_FILE` | 미설정 | `YTDLP_COOKIES_PATH`와 같은 용도의 호환 변수 |
| `YTDLP_TIMEOUT` | `30s` | 메타데이터 추출 제한 시간 |

YouTube 공개 영상은 최신 yt-dlp와 `YTDLP_JS_RUNTIME=node` 조합으로 쿠키 없이
통과할 수 있지만, IP 평판이나 요청량에 따라 429/봇 확인이 다시 발생할 수
있습니다. 이 경우 `YTDLP_PROXY`와 요청 제한을 함께 사용하세요.

YouTube가 `Sign in to confirm you're not a bot` 오류를 반환하면 브라우저에서
Netscape 형식의 `cookies.txt`를 내보내 `YTDLP_COOKIES` secret 변수에 파일
내용 전체를 넣거나, 서버 컨테이너 안에 파일로 마운트한 뒤 `YTDLP_COOKIES_PATH`
또는 `YTDLP_COOKIES_FILE`에 그 경로를 설정해야 합니다. 개인 로그인 쿠키는 계정
권한을 담고 있으므로 공개 저장소나 로그에 올리지 마세요.

## 검증

```bash
go test ./...
go vet ./...
```
