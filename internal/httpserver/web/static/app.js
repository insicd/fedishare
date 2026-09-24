const csrf = document.querySelector('meta[name="csrf-token"]')?.content || "";

async function api(path, options = {}) {
  const headers = Object.assign({ "X-CSRF-Token": csrf }, options.headers || {});
  if (options.body && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }
  const res = await fetch(path, Object.assign({}, options, { headers }));
  const text = await res.text();
  let data = {};
  try { data = text ? JSON.parse(text) : {}; } catch { data = { error: text }; }
  if (!res.ok) {
    throw new Error(data.error || res.statusText || "Request failed");
  }
  return data;
}

function $(sel) { return document.querySelector(sel); }

function formatBytes(n) {
  n = Number(n || 0);
  if (n < 1024) return n + " B";
  const units = ["KB", "MB", "GB", "TB"];
  let v = n;
  for (const u of units) {
    v /= 1024;
    if (v < 1024) return v.toFixed(1) + " " + u;
  }
  return (v / 1024).toFixed(1) + " PB";
}

function setNav() {
  if (document.querySelector(".shell") && !location.hash) {
    location.hash = "#status";
    return;
  }
  const hash = location.hash || "#status";
  document.querySelectorAll("nav a").forEach((a) => {
    a.classList.toggle("active", a.getAttribute("href") === hash);
  });
  document.querySelectorAll("main section").forEach((section) => {
    section.hidden = ("#" + section.id) !== hash;
  });
  if (hash === "#status") refreshStatus().catch(() => {});
  if (hash === "#files") refreshFiles().catch(() => {});
  if (hash === "#activity") refreshActivity().catch(() => {});
  if (hash === "#followers") refreshFollowers().catch(() => {});
  if (hash === "#about") refreshDiagnostics().catch(() => {});
  if (hash === "#profiles") refreshProfiles().catch(() => {});
}

async function refreshProfiles() {
  const data = await api("/api/profiles");
  const list = document.querySelector("[data-profile-list]");
  if (!list) return;
  const profiles = data.profiles || [];
  if (!profiles.length) {
    list.innerHTML = `<li><p class="hint">No profiles yet.</p></li>`;
    return;
  }
  list.innerHTML = profiles.map((p) => `
    <li>
      <div>
        <a class="name" href="#status" data-select-profile="${p.id}">${escapeHtml(p.display_name || p.username || "")}</a>
        <p class="meta">${escapeHtml(p.identity || "")} · ${escapeHtml(p.share_directory || "")}</p>
      </div>
      ${p.selected ? `<span class="hint">Selected</span>` : ""}
    </li>
  `).join("");
}

function escapeHtml(s) {
  return String(s || "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  }[c]));
}

async function refreshStatus() {
  const data = await api("/api/status");
  const snap = data.status || {};
  const set = (field, value) => {
    const el = document.querySelector(`[data-field="${field}"]`);
    if (el) el.textContent = value;
  };
  set("state-label", labelState(snap.state));
  set("identity", data.identity || snap.identity || "—");
  set("share", data.share_directory || "—");
  set("files", String(snap.indexed_files ?? 0));
  set("bytes", formatBytes(snap.total_bytes));
  set("gateway", snap.gateway_connected ? "Connected" : (snap.state === "connecting" || snap.state === 2 ? "Connecting" : "Disconnected"));
  set("ap", snap.activitypub_active ? "Active" : "Inactive");
  set("message", snap.message || "");
  const paused = snap.state === 4 || snap.state === "paused";
  const pauseBtn = document.querySelector('[data-action="pause"]');
  const resumeBtn = document.querySelector('[data-action="resume"]');
  if (pauseBtn && resumeBtn) {
    pauseBtn.hidden = paused;
    resumeBtn.hidden = !paused;
  }
}

function labelState(state) {
  const names = {
    0: "Starting", 1: "Indexing", 2: "Connecting", 3: "Online",
    4: "Paused", 5: "Offline", 6: "Error", 7: "Shutting down",
    starting: "Starting", indexing: "Indexing", connecting: "Connecting",
    online: "Online", paused: "Paused", offline: "Offline",
    error: "Error", shutting_down: "Shutting down"
  };
  return names[state] || String(state);
}

async function refreshFiles() {
  const body = document.querySelector("[data-file-list]");
  if (!body) return;
  try {
    const data = await api("/api/files");
    const files = data.files || [];
    if (!files.length) {
      body.innerHTML = '<tr><td colspan="5">No shared files yet. Copy something into the shared folder, then rescan.</td></tr>';
      return;
    }
    body.innerHTML = files.map((f) => {
      const href = f.download_url || ("/files/" + f.id);
      const hash = f.hash ? escapeHtml(f.hash.replace(/^fedishare:sha256:/, "sha256:").slice(0, 20)) + "…" : "—";
      return `<tr><td>${escapeHtml(f.name)}</td><td>${escapeHtml(f.relative_path)}</td><td>${formatBytes(f.size)}</td><td title="${escapeHtml(f.hash || "")}">${hash}</td><td><a href="${escapeHtml(href)}">Download</a></td></tr>`;
    }).join("");
  } catch (err) {
    body.innerHTML = `<tr><td colspan="5">${escapeHtml(err.message)}</td></tr>`;
  }
}

async function refreshActivity() {
  const body = document.querySelector("[data-activity-list]");
  if (!body) return;
  try {
    const data = await api("/api/activity");
    const items = data.items || [];
    if (!items.length) {
      body.innerHTML = '<tr><td colspan="3">No public files in the outbox yet.</td></tr>';
      return;
    }
    body.innerHTML = items.map((item) => {
      const href = item.object_url || item.download_url || "#";
      return `<tr><td>${escapeHtml(item.type || "Create")}</td><td><a href="${escapeHtml(href)}">${escapeHtml(item.name || item.id || "file")}</a></td><td>${escapeHtml(item.published || "—")}</td></tr>`;
    }).join("");
  } catch (err) {
    body.innerHTML = `<tr><td colspan="3">${escapeHtml(err.message)}</td></tr>`;
  }
}

async function refreshFollowers() {
  const body = document.querySelector("[data-follower-list]");
  if (!body) return;
  try {
    const data = await api("/api/followers");
    const items = data.items || [];
    if (!items.length) {
      body.innerHTML = '<tr><td colspan="3">Nobody is following this node yet.</td></tr>';
      return;
    }
    body.innerHTML = items.map((item) => {
      const actor = escapeHtml(item.actor_id || "");
      const host = escapeHtml(item.host || "");
      return `<tr><td><a href="${actor}">${actor}</a></td><td>${host}</td><td><button type="button" class="secondary" data-block="${actor}">Block</button></td></tr>`;
    }).join("");
    body.querySelectorAll("[data-block]").forEach((btn) => {
      btn.addEventListener("click", async () => {
        try {
          await api("/api/block", { method: "POST", body: JSON.stringify({ target: btn.getAttribute("data-block") }) });
          await refreshFollowers();
        } catch (err) {
          alert(err.message);
        }
      });
    });
  } catch (err) {
    body.innerHTML = `<tr><td colspan="3">${escapeHtml(err.message)}</td></tr>`;
  }
}

async function refreshDiagnostics() {
  const el = document.querySelector("[data-diagnostics]");
  if (!el) return;
  const data = await api("/api/diagnostics");
  el.textContent = JSON.stringify(data, null, 2);
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (ch) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  }[ch]));
}

async function copyText(text) {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  throw new Error("Clipboard is not available");
}

function bindDashboard() {
  document.querySelectorAll("[data-action]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      try {
        const action = btn.getAttribute("data-action");
        if (action === "rescan") {
          await api("/api/rescan", { method: "POST", body: "{}" });
          await refreshStatus();
          await refreshFiles();
        } else if (action === "pause") {
          await api("/api/pause", { method: "POST", body: "{}" });
          await refreshStatus();
        } else if (action === "resume") {
          await api("/api/resume", { method: "POST", body: "{}" });
          await refreshStatus();
        } else if (action === "copy-address") {
          const data = await api("/api/status");
          await copyText(data.identity || "");
        } else if (action === "copy-diag") {
          const data = await api("/api/diagnostics");
          await copyText(JSON.stringify(data, null, 2));
        }
      } catch (err) {
        alert(err.message);
      }
    });
  });

  document.addEventListener("click", async (ev) => {
    const link = ev.target.closest("[data-select-profile]");
    if (!link) return;
    ev.preventDefault();
    try {
      await api("/api/profiles/select", { method: "POST", body: JSON.stringify({ id: link.getAttribute("data-select-profile") }) });
      location.hash = "#status";
      location.reload();
    } catch (err) {
      alert(err.message);
    }
  });

  const profileForm = document.getElementById("profile-form");
  if (profileForm) {
    profileForm.addEventListener("submit", async (ev) => {
      ev.preventDefault();
      const errEl = document.getElementById("profile-error");
      const fd = new FormData(profileForm);
      try {
        await api("/api/profiles", {
          method: "POST",
          body: JSON.stringify({
            display_name: fd.get("display_name"),
            username: fd.get("username"),
            share_directory: fd.get("share_directory"),
            summary: fd.get("summary")
          })
        });
        location.hash = "#status";
        location.reload();
      } catch (err) {
        if (errEl) {
          errEl.hidden = false;
          errEl.textContent = err.message;
        }
      }
    });
  }

  const settings = document.getElementById("settings-form");
  if (settings) {
    settings.addEventListener("submit", async (ev) => {
      ev.preventDefault();
      const errEl = document.getElementById("settings-error");
      const fd = new FormData(settings);
      const payload = {
        display_name: fd.get("display_name"),
        username: fd.get("username"),
        share_directory: fd.get("share_directory"),
        gateway_url: fd.get("gateway_url"),
        summary: fd.get("summary"),
        log_level: fd.get("log_level"),
        start_at_login: fd.get("start_at_login") === "on"
      };
      try {
        await api("/api/settings", { method: "PUT", body: JSON.stringify(payload) });
        if (errEl) errEl.hidden = true;
        await refreshStatus();
      } catch (err) {
        if (errEl) {
          errEl.hidden = false;
          errEl.textContent = err.message;
        }
      }
    });
  }

  window.addEventListener("hashchange", setNav);
  setNav();
  refreshStatus().catch(() => {});
  refreshFiles().catch(() => {});
  refreshDiagnostics().catch(() => {});
  setInterval(() => { refreshStatus().catch(() => {}); }, 4000);
}

function bindWizard() {
  const form = document.getElementById("wizard-form");
  if (!form) return;
  form.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const errEl = document.getElementById("wizard-error");
    const fd = new FormData(form);
    try {
      await api("/api/setup", {
        method: "POST",
        body: JSON.stringify({
          display_name: fd.get("display_name"),
          username: fd.get("username"),
          share_directory: fd.get("share_directory"),
          gateway_url: fd.get("gateway_url"),
          summary: fd.get("summary")
        })
      });
      location.href = "/#status";
      location.reload();
    } catch (err) {
      if (errEl) {
        errEl.hidden = false;
        errEl.textContent = err.message;
      }
    }
  });
}

function bindBrowseFolders() {
  document.addEventListener("click", async (ev) => {
    const btn = ev.target.closest("[data-browse-folder]");
    if (!btn) return;
    ev.preventDefault();
    const wrap = btn.closest("label") || btn.parentElement;
    const input = wrap && wrap.querySelector('input[name="share_directory"]');
    if (!input) return;
    btn.disabled = true;
    try {
      const data = await api("/api/browse-folder", {
        method: "POST",
        body: JSON.stringify({ start: input.value || "" })
      });
      if (data.path) {
        input.value = data.path;
        input.dispatchEvent(new Event("input", { bubbles: true }));
      }
    } catch (err) {
      alert(err.message);
    } finally {
      btn.disabled = false;
    }
  });
}

bindBrowseFolders();
bindWizard();
if (document.querySelector(".shell")) {
  bindDashboard();
}
