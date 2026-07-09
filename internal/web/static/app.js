const form = document.querySelector("#url-form");
const urlInput = document.querySelector("#video-url");
const lookupButton = document.querySelector("#lookup-button");
const message = document.querySelector("#form-message");
const result = document.querySelector("#result");
const thumbnail = document.querySelector("#thumbnail");
const duration = document.querySelector("#duration");
const videoTitle = document.querySelector("#video-title");
const formatCount = document.querySelector("#format-count");
const formatList = document.querySelector("#format-list");

let currentURL = "";

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const url = urlInput.value.trim();

  if (!isSupportedURL(url)) {
    setMessage("YouTube, X 또는 TikTok의 올바른 HTTPS 주소를 입력해주세요.", true);
    urlInput.focus();
    return;
  }

  setLoading(true);
  setMessage("영상 정보를 확인하고 있습니다…");
  result.hidden = true;

  try {
    const response = await fetch("/api/v1/metadata", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url }),
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(data?.error?.message || "영상 정보를 가져오지 못했습니다.");
    }
    if (!Array.isArray(data.formats) || data.formats.length === 0) {
      throw new Error("다운로드 가능한 MP4 포맷이 없습니다.");
    }

    currentURL = url;
    renderResult(data);
    setMessage("");
    result.hidden = false;
    result.scrollIntoView({ behavior: "smooth", block: "center" });
  } catch (error) {
    setMessage(error.message || "요청 처리 중 오류가 발생했습니다.", true);
  } finally {
    setLoading(false);
  }
});

function renderResult(data) {
  videoTitle.textContent = data.title || "제목 없는 영상";
  thumbnail.src = data.thumbnail || "";
  thumbnail.alt = data.title ? `${data.title} 미리보기` : "영상 미리보기";
  duration.textContent = formatDuration(data.duration);
  duration.hidden = !data.duration;
  formatCount.textContent = `${data.formats.length}개 포맷을 사용할 수 있습니다`;
  formatList.replaceChildren();

  const sorted = [...data.formats].sort((a, b) => {
    return resolutionNumber(b.resolution) - resolutionNumber(a.resolution);
  });
  sorted.forEach((format) => formatList.append(createFormatButton(format)));
}

function createFormatButton(format) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "format-row";
  button.setAttribute("aria-label", `${format.resolution || "기본 화질"} 다운로드`);

  const quality = document.createElement("span");
  quality.className = "quality";
  const label = document.createElement("span");
  label.textContent = format.resolution || "기본 화질";
  const detail = document.createElement("small");
  detail.textContent = format.needs_merge ? "비디오 + 오디오 병합" : "비디오 · 오디오";
  quality.append(label, detail);

  const size = document.createElement("span");
  size.className = "file-size";
  size.textContent = formatFileSize(format.filesize);

  const icon = document.createElement("span");
  icon.className = "download-icon";
  icon.setAttribute("aria-hidden", "true");
  icon.innerHTML = '<svg viewBox="0 0 24 24"><path d="M12 4v11m0 0 4-4m-4 4-4-4M5 20h14"/></svg>';

  button.append(quality, size, icon);
  button.addEventListener("click", () => startDownload(format.format_id, button));
  return button;
}

function startDownload(formatID, button) {
  if (!currentURL || !formatID) return;
  const original = button.querySelector(".quality > span").textContent;
  button.querySelector(".quality > span").textContent = "다운로드 준비 중…";

  const params = new URLSearchParams({ url: currentURL, format_id: formatID });
  window.location.assign(`/api/v1/download?${params.toString()}`);

  window.setTimeout(() => {
    button.querySelector(".quality > span").textContent = original;
  }, 3500);
}

function isSupportedURL(value) {
  try {
    const url = new URL(value);
    if (url.protocol !== "https:") return false;
    const host = url.hostname.toLowerCase().replace(/\.$/, "");
    return ["youtube.com", "youtu.be", "x.com", "twitter.com", "tiktok.com"]
      .some((allowed) => host === allowed || host.endsWith(`.${allowed}`));
  } catch {
    return false;
  }
}

function formatDuration(seconds) {
  if (!Number.isFinite(seconds) || seconds <= 0) return "";
  const total = Math.round(seconds);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const remain = total % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(remain).padStart(2, "0")}`
    : `${minutes}:${String(remain).padStart(2, "0")}`;
}

function formatFileSize(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "크기 미상";
  const units = ["B", "KB", "MB", "GB"];
  const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / (1024 ** unit)).toFixed(unit > 1 ? 1 : 0)} ${units[unit]}`;
}

function resolutionNumber(value = "") {
  const match = String(value).match(/\d+/);
  return match ? Number(match[0]) : 0;
}

function setLoading(loading) {
  lookupButton.disabled = loading;
  lookupButton.querySelector("span").textContent = loading ? "확인 중…" : "불러오기";
}

function setMessage(text, isError = false) {
  message.textContent = text;
  message.classList.toggle("error", isError);
}
