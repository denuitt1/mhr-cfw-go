"use strict";

const SECTIONS = [
    {
        title: "Authentication",
        fields: [
            { key: "auth_key",       def: "",  sensitive: true },
            { key: "deployment_ids", def: [],  multiline: true },
            { key: "parallel_relay", def: 1 },
        ],
    },
    {
        title: "Listeners",
        fields: [
            { key: "listen_host",    def: "127.0.0.1" },
            { key: "listen_port",    def: 8080 },
            { key: "socks5_enabled", def: true },
            { key: "socks5_port",    def: 1080 },
            { key: "lan_sharing",    def: false },
        ],
    },
    {
        title: "Domain fronting",
        fields: [
            { key: "mode",         def: "relay" },
            { key: "front_domain", def: "www.google.com" },
            { key: "google_ip",    def: "216.239.38.120" },
            { key: "verify_ssl",   def: true },
            { key: "hosts",        def: [] },
        ],
    },
    {
        title: "Worker + upstream",
        fields: [
            { key: "worker_url",             def: "" },
            { key: "upstream_forwarder_url", def: "" },
        ],
    },
    {
        title: "Web GUI",
        fields: [
            { key: "web_enabled", def: true },
            { key: "web_port",    def: 8081 },
            { key: "probe_url",   def: "https://www.gstatic.com/generate_204" },
        ],
    },
    {
        title: "Advanced",
        fields: [
            { key: "log_level",                      def: "INFO" },
            { key: "relay_timeout",                  def: 25000 },
            { key: "tcp_connect_timeout",            def: 10000 },
            { key: "tls_connect_timeout",            def: 15000 },
            { key: "max_request_body_bytes",         def: 104857600 },
            { key: "max_response_body_bytes",        def: 209715200 },
            { key: "chunked_download_chunk_size",    def: 524288 },
            { key: "chunked_download_max_parallel",  def: 8 },
            { key: "chunked_download_min_size",      def: 5242880 },
        ],
    },
];

const FIELDS = SECTIONS.flatMap(s => s.fields);

let originalConfig = null;

async function fetchJSON(url, options) {
    const resp = await fetch(url, options);
    const text = await resp.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (_) {}
    if (!resp.ok) {
        const msg = (data && data.error) || text || resp.statusText;
        throw new Error(msg);
    }
    return data;
}

function inputForField(field, value) {
    const wrap = document.createElement("div");
    wrap.className = "row-input";

    const def = field.def;
    if (typeof def === "boolean") {
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.name = field.key;
        cb.checked = typeof value === "boolean" ? value : def;
        wrap.appendChild(cb);
        return wrap;
    }

    if (Array.isArray(def)) {
        const arr = Array.isArray(value) ? value : def;
        if (field.multiline) {
            const ta = document.createElement("textarea");
            ta.name = field.key;
            ta.rows = Math.max(3, arr.length || 0);
            ta.value = arr.join("\n");
            ta.placeholder = "one per line";
            ta.dataset.kind = "list-lines";
            wrap.appendChild(ta);
            return wrap;
        }
        const inp = document.createElement("input");
        inp.type = "text";
        inp.name = field.key;
        inp.value = arr.join(", ");
        inp.placeholder = "comma-separated";
        inp.dataset.kind = "list";
        wrap.appendChild(inp);
        return wrap;
    }

    const inp = document.createElement("input");
    if (typeof def === "number") {
        inp.type = "number";
        inp.dataset.kind = "number";
        inp.value = (typeof value === "number") ? String(value) : String(def);
    } else {
        inp.type = field.sensitive ? "password" : "text";
        if (field.sensitive) inp.autocomplete = "off";
        inp.value = (typeof value === "string") ? value : (def == null ? "" : String(def));
    }
    inp.name = field.key;
    wrap.appendChild(inp);
    return wrap;
}

function renderForm(cfg) {
    const container = document.getElementById("form-sections");
    container.innerHTML = "";
    const data = (cfg && cfg.config) || {};

    const tabs = document.createElement("div");
    tabs.className = "tabs";
    tabs.setAttribute("role", "tablist");

    const panels = document.createElement("div");
    panels.className = "tab-panels";

    SECTIONS.forEach((section, idx) => {
        const id = "tab-" + idx;
        const panelId = "panel-" + idx;

        const tab = document.createElement("button");
        tab.type = "button";
        tab.className = "tab";
        tab.id = id;
        tab.setAttribute("role", "tab");
        tab.setAttribute("aria-controls", panelId);
        tab.setAttribute("aria-selected", idx === 0 ? "true" : "false");
        tab.tabIndex = idx === 0 ? 0 : -1;
        tab.textContent = section.title;
        tab.addEventListener("click", () => activateTab(idx));
        tabs.appendChild(tab);

        const panel = document.createElement("div");
        panel.className = "tab-panel";
        panel.id = panelId;
        panel.setAttribute("role", "tabpanel");
        panel.setAttribute("aria-labelledby", id);
        if (idx !== 0) panel.hidden = true;

        const grid = document.createElement("div");
        grid.className = "form-grid";
        for (const field of section.fields) {
            const label = document.createElement("label");
            label.className = "row-label";
            label.textContent = field.key;
            label.htmlFor = "f-" + field.key;
            grid.appendChild(label);

            const input = inputForField(field, data[field.key]);
            const el = input.querySelector("input, textarea");
            if (el) el.id = "f-" + field.key;
            grid.appendChild(input);
        }
        panel.appendChild(grid);
        panels.appendChild(panel);
    });

    container.appendChild(tabs);
    container.appendChild(panels);
}

function activateTab(activeIdx) {
    const tabs = document.querySelectorAll("#form-sections .tab");
    const panels = document.querySelectorAll("#form-sections .tab-panel");
    tabs.forEach((t, i) => {
        const on = i === activeIdx;
        t.setAttribute("aria-selected", on ? "true" : "false");
        t.tabIndex = on ? 0 : -1;
    });
    panels.forEach((p, i) => {
        p.hidden = i !== activeIdx;
    });
}

function readFormInto(base) {
    const out = base ? JSON.parse(JSON.stringify(base)) : {};
    out.config = out.config || {};
    for (const field of FIELDS) {
        const inp = document.querySelector(
            '#form-sections [name="' + field.key + '"]'
        );
        if (!inp) continue;
        if (inp.type === "checkbox") {
            out.config[field.key] = inp.checked;
        } else if (inp.dataset.kind === "list-lines") {
            out.config[field.key] = inp.value
                .split(/\r?\n/)
                .map(s => s.trim())
                .filter(Boolean);
        } else if (inp.dataset.kind === "list") {
            out.config[field.key] = inp.value
                .split(",")
                .map(s => s.trim())
                .filter(Boolean);
        } else if (inp.dataset.kind === "number") {
            const v = inp.value === "" ? field.def : Number(inp.value);
            out.config[field.key] = Number.isFinite(v) ? v : field.def;
        } else {
            out.config[field.key] = inp.value;
        }
    }
    return out;
}

async function loadConfig() {
    let cfg = null;
    try {
        cfg = await fetchJSON("/api/config");
    } catch (err) {
        console.warn("config fetch failed, rendering defaults:", err);
    }
    originalConfig = cfg || {};
    renderForm(originalConfig);
    updateProbeDescriptions(originalConfig.config || {});
}

function updateProbeDescriptions(c) {
    const detail = document.getElementById("worker-probe-detail");
    if (!detail) return;
    if (c.worker_url) {
        const upstreamNote = c.upstream_forwarder_url ? " + upstream forwarder root" : "";
        detail.innerHTML = 'Direct GET to <code>worker_url</code>' + upstreamNote;
    } else {
        const probeUrl = c.probe_url || "https://www.gstatic.com/generate_204";
        detail.innerHTML = 'Full relay to <code>' + probeUrl + '</code>';
    }
}

async function loadVersion() {
    try {
        const v = await fetchJSON("/api/version");
        if (v) {
            document.getElementById("app-title").textContent = v.name || "MHR-CFW";
            document.getElementById("app-version").textContent = v.version ? "v" + v.version : "";
        }
    } catch (_) {}
}

function setProbeRow(key, state, latencyMs, errMsg) {
    const row = document.querySelector('.probe[data-key="' + key + '"]');
    if (!row) return;
    const dot = row.querySelector(".dot");
    const status = row.querySelector(".probe-status");
    dot.className = "dot dot-" + state;
    status.classList.remove("ok", "err");
    if (state === "loading") {
        status.textContent = "checking…";
    } else if (state === "ok") {
        status.textContent = latencyMs + " ms";
        status.classList.add("ok");
    } else if (state === "err") {
        const msg = errMsg || "failed";
        status.textContent = msg.length > 240 ? msg.slice(0, 240) + "…" : msg;
        status.classList.add("err");
    } else {
        status.textContent = "—";
    }
}

async function runProbes() {
    const btn = document.getElementById("probe-btn");
    btn.disabled = true;
    for (const k of ["google", "gas", "worker"]) setProbeRow(k, "loading");
    try {
        const report = await fetchJSON("/api/probe", { method: "POST" });
        for (const k of ["google", "gas", "worker"]) {
            const r = report[k] || {};
            if (r.ok) setProbeRow(k, "ok", r.latency_ms);
            else setProbeRow(k, "err", r.latency_ms, r.error);
        }
    } catch (err) {
        for (const k of ["google", "gas", "worker"]) setProbeRow(k, "err", 0, err.message);
    } finally {
        btn.disabled = false;
    }
}

async function saveConfig(ev) {
    ev.preventDefault();
    const msg = document.getElementById("form-msg");
    msg.className = "muted";
    msg.textContent = "saving…";
    const merged = readFormInto(originalConfig);
    try {
        await fetchJSON("/api/config", {
            method: "PUT",
            headers: { "content-type": "application/json" },
            body: JSON.stringify(merged),
        });
        msg.className = "ok";
        msg.textContent = "saved — proxy restarting";
        await loadConfig();
        setTimeout(runProbes, 1500);
    } catch (err) {
        msg.className = "err";
        msg.textContent = err.message;
    }
}

document.getElementById("probe-btn").addEventListener("click", runProbes);
document.getElementById("config-form").addEventListener("submit", saveConfig);

(async () => {
    await loadVersion();
    await loadConfig();
    runProbes();
})();
