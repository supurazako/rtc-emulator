"use strict";

const roomInput = document.getElementById("room");
const wsInput = document.getElementById("ws");
const joinButton = document.getElementById("join");
const leaveButton = document.getElementById("leave");
const statusEl = document.getElementById("status");

const panes = [
  {
    name: "good",
    video: document.getElementById("good-video"),
    rtt: document.getElementById("good-rtt"),
    bitrate: document.getElementById("good-bitrate"),
  },
  {
    name: "medium",
    video: document.getElementById("medium-video"),
    rtt: document.getElementById("medium-rtt"),
    bitrate: document.getElementById("medium-bitrate"),
  },
  {
    name: "bad",
    video: document.getElementById("bad-video"),
    rtt: document.getElementById("bad-rtt"),
    bitrate: document.getElementById("bad-bitrate"),
  },
];

let clients = [];
let statsTimer = null;

function setStatus(message) {
  statusEl.textContent = `Status: ${message}`;
}

function setIdleStats() {
  panes.forEach((pane) => {
    pane.rtt.textContent = "RTT: -";
    pane.bitrate.textContent = "Bitrate: -";
  });
}

function parseCandidatePairs(report) {
  for (const stat of report.values()) {
    if (stat.type !== "candidate-pair") {
      continue;
    }
    if (stat.state !== "succeeded") {
      continue;
    }
    if (typeof stat.currentRoundTripTime === "number") {
      return `${Math.round(stat.currentRoundTripTime * 1000)} ms`;
    }
  }
  return "-";
}

function parseBitrate(report, previousBytes, elapsedMs) {
  let bytes = 0;
  for (const stat of report.values()) {
    if (stat.type === "inbound-rtp" && stat.kind === "video" && typeof stat.bytesReceived === "number") {
      bytes += stat.bytesReceived;
    }
  }
  if (bytes === 0 || previousBytes < 0 || elapsedMs <= 0) {
    return { value: "-", bytes };
  }
  const bps = ((bytes - previousBytes) * 8 * 1000) / elapsedMs;
  return { value: `${Math.max(0, Math.round(bps / 1000))} kbps`, bytes };
}

async function updateStats() {
  const now = Date.now();
  for (const pane of panes) {
    const entry = clients.find((c) => c.pane === pane.name);
    if (!entry || !entry.pc || typeof entry.pc.getStats !== "function") {
      pane.rtt.textContent = "RTT: -";
      pane.bitrate.textContent = "Bitrate: -";
      continue;
    }
    try {
      const report = await entry.pc.getStats();
      pane.rtt.textContent = `RTT: ${parseCandidatePairs(report)}`;
      const elapsedMs = entry.lastStatsAt > 0 ? now - entry.lastStatsAt : 0;
      const bitrate = parseBitrate(report, entry.lastBytes, elapsedMs);
      pane.bitrate.textContent = `Bitrate: ${bitrate.value}`;
      entry.lastBytes = bitrate.bytes;
      entry.lastStatsAt = now;
    } catch (_) {
      pane.rtt.textContent = "RTT: -";
      pane.bitrate.textContent = "Bitrate: -";
    }
  }
}

function startStats() {
  stopStats();
  statsTimer = setInterval(() => {
    void updateStats();
  }, 1000);
}

function stopStats() {
  if (statsTimer !== null) {
    clearInterval(statsTimer);
    statsTimer = null;
  }
}

function detectPeerConnection(client) {
  if (!client || typeof client !== "object") {
    return null;
  }
  // ion-sdk internals differ by version; this probes common shapes.
  if (client.pc && typeof client.pc.getStats === "function") {
    return client.pc;
  }
  if (client.transports && client.transports.sub && client.transports.sub.pc) {
    return client.transports.sub.pc;
  }
  if (client.transports && client.transports[0] && client.transports[0].pc) {
    return client.transports[0].pc;
  }
  return null;
}

async function joinPane(paneName, uid, room, wsURL) {
  if (!window.ion || !window.ion.SFU || !window.ion.Signal) {
    throw new Error("ion-sdk-js is not available in this browser context");
  }

  const signal = new window.ion.Signal.IonSFUJSONRPCSignal(wsURL);
  const client = new window.ion.SFU.Client(signal);
  let streamSet = false;

  client.ontrack = (track, stream) => {
    const pane = panes.find((p) => p.name === paneName);
    if (!pane || !stream) {
      return;
    }
    if (!streamSet) {
      pane.video.srcObject = stream;
      streamSet = true;
    }
    if (track && typeof track.onended !== "undefined") {
      track.onended = () => {
        pane.video.srcObject = null;
      };
    }
  };

  await client.join(room, uid);
  return client;
}

async function joinAll() {
  const room = roomInput.value.trim();
  const wsURL = wsInput.value.trim();
  if (room === "" || wsURL === "") {
    setStatus("room and SFU WS are required");
    return;
  }

  setStatus("joining...");
  joinButton.disabled = true;
  leaveButton.disabled = false;
  setIdleStats();

  try {
    const joined = [];
    for (const pane of panes) {
      const uid = `${pane.name}-${Math.random().toString(36).slice(2, 8)}`;
      const client = await joinPane(pane.name, uid, room, wsURL);
      joined.push({
        pane: pane.name,
        client,
        pc: detectPeerConnection(client),
        lastBytes: -1,
        lastStatsAt: 0,
      });
    }
    clients = joined;
    startStats();
    setStatus("connected (waiting tracks)");
  } catch (err) {
    setStatus(`join failed: ${err.message || String(err)}`);
    await leaveAll();
  }
}

async function leaveAll() {
  stopStats();
  for (const entry of clients) {
    try {
      if (entry.client && typeof entry.client.close === "function") {
        entry.client.close();
      }
    } catch (_) {
      // no-op
    }
  }
  clients = [];
  panes.forEach((pane) => {
    pane.video.srcObject = null;
  });
  setIdleStats();
  joinButton.disabled = false;
  leaveButton.disabled = true;
  setStatus("idle");
}

joinButton.addEventListener("click", () => {
  void joinAll();
});

leaveButton.addEventListener("click", () => {
  void leaveAll();
});
