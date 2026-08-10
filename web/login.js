const elements = {
  statusDot: document.querySelector("#status-dot"),
  statusLabel: document.querySelector("#status-label"),
  brandLabel: document.querySelector("#brand-label"),
  qrFrame: document.querySelector("#qr-frame"),
  qrImage: document.querySelector("#qr-image"),
  qrPlaceholder: document.querySelector("#qr-placeholder"),
  stageTitle: document.querySelector("#stage-title"),
  stageDetail: document.querySelector("#stage-detail"),
  sessionTime: document.querySelector("#session-time"),
  retryButton: document.querySelector("#retry-button"),
};

const stages = {
  login_qrcode: {
    status: "Login code ready",
    title: "Scan with Xiaohongshu",
    detail: "Open the Xiaohongshu app, scan this code, then confirm the sign-in on your phone.",
  },
  waiting_confirmation: {
    status: "Waiting for phone confirmation",
    title: "Confirm on your phone",
    detail: "The code was scanned. Approve the sign-in in the Xiaohongshu app and keep this page open.",
  },
  device_verification: {
    status: "Device verification required",
    title: "Scan the verification code",
    detail: "Xiaohongshu requested an additional device check. Scan this new code with the app.",
  },
  logged_in: {
    status: "Signed in",
    title: "Login complete",
    detail: "The site session was restored in a fresh browser. You can close this page and use Xiaohongshu from Copilot CLI.",
  },
  verifying: {
    status: "Verifying saved login",
    title: "Checking the saved session",
    detail: "The scan succeeded. The service is reopening the site with exported cookies before reporting final success.",
  },
  persistence_failed: {
    status: "Login could not be restored",
    title: "The saved site session is not signed in",
    detail: "The live browser looked signed in, but the exported site session failed verification. Start a new session to retry.",
  },
  unknown: {
    status: "Checking login state",
    title: "Sign-in is still progressing",
    detail: "The browser is between steps. Keep this page open while the next state is detected.",
  },
  idle: {
    status: "No active login session",
    title: "Ready to start again",
    detail: "The previous session ended or expired. Start a new session to generate a fresh code.",
  },
};

let pollTimer;
let requestInFlight = false;
let lastFingerprint = "";
let lastProgressAt = Date.now();

function setStatusClass(name) {
  elements.statusDot.className = `status-dot ${name}`;
}

function setLoading(message) {
  setStatusClass("is-working");
  elements.statusLabel.textContent = message;
  elements.stageTitle.textContent = "Starting sign-in";
  elements.stageDetail.textContent =
    "The first launch can take up to a minute while the protected browser starts.";
  elements.sessionTime.textContent = "";
  elements.qrImage.hidden = true;
  elements.qrPlaceholder.hidden = false;
  elements.retryButton.hidden = true;
}

function imageSource(value) {
  if (!value) return "";
  return value.startsWith("data:") ? value : `data:image/png;base64,${value}`;
}

function render(data) {
  const copy = stages[data.stage] || stages.unknown;
  const productName = data.site === "rednote" ? "RedNote" : "Xiaohongshu";
  elements.brandLabel.textContent = `${productName} read-only`;
  const fingerprint = `${data.stage}:${data.img || ""}`;
  if (fingerprint !== lastFingerprint) {
    lastFingerprint = fingerprint;
    lastProgressAt = Date.now();
  }

  elements.statusLabel.textContent = copy.status;
  elements.stageTitle.textContent = copy.title.replaceAll("Xiaohongshu", productName);
  elements.stageDetail.textContent = copy.detail.replaceAll("Xiaohongshu", productName);
  elements.retryButton.hidden = !["idle", "persistence_failed"].includes(data.stage);

  if (data.is_logged_in) {
    setStatusClass("is-ready");
  } else if (data.stage === "persistence_failed") {
    setStatusClass("is-error");
  } else if (data.stage === "idle") {
    setStatusClass("");
  } else {
    setStatusClass("is-working");
  }

  const source = imageSource(data.img);
  if (source) {
    if (elements.qrImage.src !== source) {
      elements.qrImage.src = source;
    }
    elements.qrImage.hidden = false;
    elements.qrPlaceholder.hidden = true;
  } else {
    elements.qrImage.hidden = true;
    elements.qrPlaceholder.hidden = false;
    elements.qrPlaceholder.querySelector("span:last-child").textContent =
      data.is_logged_in
        ? "Site session verified"
        : data.stage === "verifying"
          ? "Reopening the saved site session..."
          : "Waiting for the next login step...";
  }

  if (
    data.active &&
    Date.now() - lastProgressAt > 45_000 &&
    data.stage === "waiting_confirmation"
  ) {
    elements.sessionTime.textContent =
      "Still waiting. Check the Xiaohongshu app for a confirmation prompt.";
  } else if (data.active && data.timeout && data.timeout !== "0s") {
    elements.sessionTime.textContent = `This QR login attempt expires in about ${data.timeout}.`;
  } else {
    elements.sessionTime.textContent = "";
  }
}

async function requestSession(method) {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), method === "POST" ? 75_000 : 10_000);
  try {
    const response = await fetch("/api/v1/login/session", {
      method,
      cache: "no-store",
      headers: method === "POST" ? { "Content-Type": "application/json" } : undefined,
      signal: controller.signal,
    });
    const body = await response.json();
    if (!response.ok || !body.success) {
      throw new Error(body.error || `Request failed with status ${response.status}`);
    }
    return body.data;
  } finally {
    window.clearTimeout(timeout);
  }
}

function showError(error) {
  window.clearTimeout(pollTimer);
  setStatusClass("is-error");
  elements.statusLabel.textContent = "Login session error";
  elements.stageTitle.textContent = "The local login flow stopped";
  elements.stageDetail.textContent =
    error.name === "AbortError"
      ? "The browser took too long to respond. Start a new session and try again."
      : error.message;
  elements.sessionTime.textContent = "";
  elements.qrImage.hidden = true;
  elements.qrPlaceholder.hidden = false;
  elements.qrPlaceholder.querySelector("span:last-child").textContent =
    "No QR code is currently available.";
  elements.retryButton.hidden = false;
}

function schedulePoll(delay = 1500) {
  window.clearTimeout(pollTimer);
  pollTimer = window.setTimeout(poll, delay);
}

async function poll() {
  if (requestInFlight || document.hidden) {
    schedulePoll(1000);
    return;
  }

  requestInFlight = true;
  try {
    const data = await requestSession("GET");
    render(data);
    if (!data.is_logged_in && !["idle", "persistence_failed"].includes(data.stage)) {
      schedulePoll();
    }
  } catch (error) {
    showError(error);
  } finally {
    requestInFlight = false;
  }
}

async function start() {
  if (requestInFlight) return;

  requestInFlight = true;
  elements.retryButton.disabled = true;
  setLoading("Starting local login session");
  try {
    const data = await requestSession("POST");
    render(data);
    if (!data.is_logged_in) {
      schedulePoll();
    }
  } catch (error) {
    showError(error);
  } finally {
    requestInFlight = false;
    elements.retryButton.disabled = false;
  }
}

elements.retryButton.addEventListener("click", start);
document.addEventListener("visibilitychange", () => {
  if (!document.hidden) poll();
});

start();
