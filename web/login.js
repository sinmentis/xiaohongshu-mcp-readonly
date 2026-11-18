const elements = {
  connection: document.querySelector("#connection"),
  statusDot: document.querySelector("#status-dot"),
  statusLabel: document.querySelector("#status-label"),
  brandLabel: document.querySelector("#brand-label"),
  qrImage: document.querySelector("#qr-image"),
  qrPlaceholder: document.querySelector("#qr-placeholder"),
  stageTitle: document.querySelector("#stage-title"),
  stageDetail: document.querySelector("#stage-detail"),
  sessionTime: document.querySelector("#session-time"),
  retryButton: document.querySelector("#retry-button"),
  serverAddress: document.querySelector("#server-address"),
};

const stages = {
  login_qrcode: {
    status: "Waiting for scan",
    title: "Sign in to {site}.",
    detail: "Scan with the {site} app, then confirm on your phone.",
  },
  waiting_confirmation: {
    status: "Waiting for confirmation",
    title: "Check your phone.",
    detail: "Approve the sign-in in the {site} app.",
  },
  device_verification: {
    status: "One more step",
    title: "Verify your device.",
    detail: "Scan this new code with the {site} app.",
  },
  logged_in: {
    status: "Connected",
    title: "You're connected.",
    detail: "{site} is ready. You can close this page.",
  },
  verifying: {
    status: "Verifying saved session",
    title: "Almost there.",
    detail: "Checking that your saved sign-in works.",
  },
  persistence_failed: {
    status: "Session not saved",
    title: "Let's try again.",
    detail: "Your sign-in could not be restored. Scan a new code.",
    action: "Sign in again",
  },
  unknown: {
    status: "Checking connection",
    title: "Just a moment.",
    detail: "Waiting for the next sign-in step.",
  },
  idle: {
    status: "Session ended",
    title: "Sign in to {site}.",
    detail: "Start a new session to get a fresh code.",
    action: "Get a new code",
  },
};

const errors = {
  BROWSER_BUSY: "The browser is in use. Wait a moment, then try again.",
  BROWSER_UNAVAILABLE: "The local browser is unavailable. Restart the server and try again.",
  QUEUE_TIMEOUT: "The browser is busy. Wait a moment, then try again.",
  RATE_LIMITED: "Please wait a moment before trying again.",
  OPERATION_TIMEOUT: "The site took too long to respond. Try again.",
  LOGIN_SESSION_FAILED: "Could not open sign-in. Check the local server and try again.",
};

let pollTimer;
let requestInFlight = false;
let productName = "Xiaohongshu / RedNote";

elements.serverAddress.textContent = window.location.host;

function setState(state, statusClass) {
  elements.connection.dataset.state = state;
  elements.connection.setAttribute("aria-busy", String(["loading", "verifying", "unknown"].includes(state)));
  elements.statusDot.className = `status-dot ${statusClass}`;
}

function setImage(source = "") {
  elements.qrImage.hidden = !source;
  elements.qrPlaceholder.hidden = !!source;
  if (source) {
    if (elements.qrImage.getAttribute("src") !== source) {
      elements.qrImage.src = source;
    }
  } else {
    elements.qrImage.removeAttribute("src");
  }
}

function setLoading() {
  window.clearTimeout(pollTimer);
  setState("loading", "is-working");
  elements.statusLabel.textContent = "Starting sign-in";
  elements.stageTitle.textContent = "Connecting.";
  elements.stageDetail.textContent = "Opening the local browser. This may take a minute.";
  elements.sessionTime.textContent = "";
  elements.retryButton.hidden = true;
  setImage();
}

function render(data) {
  const stage = data.is_logged_in ? "logged_in" : (stages[data.stage] ? data.stage : "unknown");
  const copy = stages[stage];
  if (data.site === "rednote" || data.site === "xiaohongshu") {
    productName = data.site === "rednote" ? "RedNote" : "Xiaohongshu";
  }
  elements.brandLabel.textContent = `${productName} MCP`;
  elements.qrImage.alt = `${productName} sign-in QR code`;
  document.title = `${stage === "logged_in" ? "Connected" : "Connect account"} · ${productName} MCP`;
  elements.stageTitle.textContent = copy.title.replaceAll("{site}", productName);
  elements.stageDetail.textContent = copy.detail.replaceAll("{site}", productName);
  elements.statusLabel.textContent = copy.status;
  elements.retryButton.hidden = !copy.action;
  elements.retryButton.textContent = copy.action || "Try again";
  setState(stage, stage === "logged_in" ? "is-ready" : stage === "persistence_failed" ? "is-error" : stage === "idle" ? "" : "is-working");

  const canShowQR = ["login_qrcode", "device_verification"].includes(stage);
  const source = canShowQR && data.img ? (data.img.startsWith("data:") ? data.img : `data:image/png;base64,${data.img}`) : "";
  setImage(source);
  elements.sessionTime.textContent = data.active && source && data.timeout && data.timeout !== "0s"
    ? `Code session expires in ${data.timeout}.`
    : "";
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
      throw new Error(errors[body.code] || "Could not connect. Check the local server and try again.");
    }
    return body.data;
  } finally {
    window.clearTimeout(timeout);
  }
}

function showError(error) {
  window.clearTimeout(pollTimer);
  setState("error", "is-error");
  elements.statusLabel.textContent = "Connection interrupted";
  elements.stageTitle.textContent = "Couldn't connect.";
  elements.stageDetail.textContent = error.name === "AbortError"
    ? "The browser took too long. Try again."
    : error instanceof TypeError || error instanceof SyntaxError
      ? "The local server is unavailable. Check that it is running."
      : error.message;
  elements.sessionTime.textContent = "";
  elements.retryButton.textContent = "Try again";
  elements.retryButton.hidden = false;
  setImage();
}

function schedulePoll(delay = 1500) {
  window.clearTimeout(pollTimer);
  pollTimer = window.setTimeout(() => loadSession(false), delay);
}

async function loadSession(createIfIdle) {
  if (requestInFlight) return;
  if (!createIfIdle && document.hidden) {
    schedulePoll();
    return;
  }

  requestInFlight = true;
  elements.retryButton.disabled = true;
  if (createIfIdle) setLoading();
  try {
    let data = await requestSession("GET");
    if (createIfIdle && data.stage === "idle" && !data.is_logged_in) {
      data = await requestSession("POST");
    }
    render(data);
    if (!data.is_logged_in && !["idle", "persistence_failed"].includes(data.stage)) {
      schedulePoll();
    }
  } catch (error) {
    showError(error);
  } finally {
    requestInFlight = false;
    elements.retryButton.disabled = false;
  }
}

elements.retryButton.addEventListener("click", async () => {
  if (requestInFlight) return;
  requestInFlight = true;
  elements.retryButton.disabled = true;
  setLoading();
  try {
    const data = await requestSession("POST");
    render(data);
    if (!data.is_logged_in && !["idle", "persistence_failed"].includes(data.stage)) schedulePoll();
  } catch (error) {
    showError(error);
  } finally {
    requestInFlight = false;
    elements.retryButton.disabled = false;
  }
});
elements.qrImage.addEventListener("error", () => {
  showError(new Error("The code could not be displayed. Try again for a new code."));
});
document.addEventListener("visibilitychange", () => {
  if (!document.hidden && !["logged_in", "error", "persistence_failed", "idle"].includes(elements.connection.dataset.state)) {
    window.clearTimeout(pollTimer);
    loadSession(false);
  }
});

loadSession(true);
