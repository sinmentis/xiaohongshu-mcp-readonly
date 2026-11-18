const { test } = require("node:test");
const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { runInNewContext } = require("node:vm");

const source = readFileSync(join(__dirname, "login.js"), "utf8");

function createPage(responses) {
  const elements = new Map();
  const events = {};
  const requests = [];
  const timers = new Map();
  let timerID = 0;
  const document = {
    hidden: false,
    title: "",
    querySelector(selector) {
      if (!elements.has(selector)) {
        const attributes = new Map();
        elements.set(selector, {
          dataset: {},
          hidden: true,
          disabled: false,
          textContent: "",
          setAttribute(key, value) { attributes.set(key, value); },
          getAttribute(key) { return attributes.get(key) ?? null; },
          removeAttribute(key) { attributes.delete(key); },
          set src(value) { attributes.set("src", value); },
          addEventListener(name, callback) { events[`${selector}:${name}`] = callback; },
        });
      }
      return elements.get(selector);
    },
    addEventListener(name, callback) { events[name] = callback; },
  };
  const window = {
    location: { host: "127.0.0.1:18060" },
    setTimeout(callback, delay) {
      timers.set(++timerID, { callback, delay });
      return timerID;
    },
    clearTimeout(id) { timers.delete(id); },
  };
  runInNewContext(source, {
    document, window, AbortController, Error, TypeError, SyntaxError,
    fetch: async (_url, options) => {
      requests.push(options.method);
      const response = responses.shift();
      assert.ok(response, "Unexpected login request");
      if (response instanceof Error) throw response;
      return { ok: response.success !== false, json: async () => response };
    },
  });
  return {
    document, requests, events,
    element: (id) => document.querySelector(`#${id}`),
    async settle() { await new Promise(setImmediate); },
    async poll() {
      const entry = [...timers.entries()].find(([, timer]) => timer.delay === 1500);
      assert.ok(entry, "Expected a status poll");
      timers.delete(entry[0]);
      await entry[1].callback();
    },
    get hasPoll() { return [...timers.values()].some((timer) => timer.delay === 1500); },
  };
}

function session(stage, overrides = {}) {
  return {
    success: true,
    data: {
      stage, site: "rednote",
      is_logged_in: stage === "logged_in",
      active: !["idle", "logged_in", "persistence_failed"].includes(stage),
      ...overrides,
    },
  };
}

test("reopening a connected page reuses the saved state without a new login", async () => {
  const page = createPage([session("logged_in")]);
  await page.settle();
  assert.deepEqual(page.requests, ["GET"]);
  assert.equal(page.element("connection").dataset.state, "logged_in");
  assert.equal(page.element("status-label").textContent, "Connected");
  assert.equal(page.element("qr-image").hidden, true);
  assert.equal(page.hasPoll, false);
  assert.equal(page.document.title, "Connected · RedNote MCP");
});

test("an idle session starts once and displays its QR code", async () => {
  const page = createPage([
    session("idle"),
    session("login_qrcode", { img: "data:image/png;base64,initial", timeout: "9m" }),
  ]);
  await page.settle();
  assert.deepEqual(page.requests, ["GET", "POST"]);
  assert.equal(page.element("qr-image").hidden, false);
  assert.equal(page.element("qr-image").getAttribute("src"), "data:image/png;base64,initial");
  assert.equal(page.element("qr-image").alt, "RedNote sign-in QR code");
  assert.equal(page.hasPoll, true);
});

test("device verification replaces the code and unverified success never looks connected", async () => {
  const page = createPage([
    session("login_qrcode", { img: "initial" }),
    session("device_verification", { img: "device" }),
    session("verifying", { img: "stale" }),
    session("logged_in"),
  ]);
  await page.settle();
  assert.deepEqual(page.requests, ["GET"]);
  await page.poll();
  assert.equal(page.element("qr-image").getAttribute("src"), "data:image/png;base64,device");
  assert.equal(page.element("stage-title").textContent, "Verify your device.");
  await page.poll();
  assert.equal(page.element("connection").dataset.state, "verifying");
  assert.equal(page.element("connection").getAttribute("aria-busy"), "true");
  assert.equal(page.element("qr-image").getAttribute("src"), null);
  await page.poll();
  assert.equal(page.element("connection").dataset.state, "logged_in");
  assert.equal(page.hasPoll, false);
});

test("network failure gives a recovery action and retry can connect", async () => {
  const page = createPage([new TypeError("fetch failed"), session("logged_in")]);
  await page.settle();
  assert.equal(page.element("connection").dataset.state, "error");
  assert.match(page.element("stage-detail").textContent, /local server is unavailable/);
  assert.equal(page.element("retry-button").hidden, false);
  assert.equal(page.element("retry-button").disabled, false);
  assert.equal(page.hasPoll, false);
  await page.events["#retry-button:click"]();
  assert.deepEqual(page.requests, ["GET", "POST"]);
  assert.equal(page.element("connection").dataset.state, "logged_in");
});

test("failed persistence stops polling and offers an explicit fresh sign-in", async () => {
  const page = createPage([
    session("persistence_failed"),
    session("login_qrcode", { img: "fresh" }),
  ]);
  await page.settle();
  assert.deepEqual(page.requests, ["GET"]);
  assert.equal(page.hasPoll, false);
  assert.equal(page.element("retry-button").textContent, "Sign in again");
  await page.events["#retry-button:click"]();
  assert.equal(page.element("qr-image").getAttribute("src"), "data:image/png;base64,fresh");
});

test("image failure clears the code and exposes retry instead of a broken QR", async () => {
  const page = createPage([session("login_qrcode", { img: "broken" })]);
  await page.settle();
  page.events["#qr-image:error"]();
  assert.equal(page.element("connection").dataset.state, "error");
  assert.equal(page.element("qr-image").getAttribute("src"), null);
  assert.equal(page.element("qr-image").hidden, true);
  assert.equal(page.element("retry-button").hidden, false);
  assert.equal(page.hasPoll, false);
});
