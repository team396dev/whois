'use strict';

const $ = id => document.getElementById(id);

const elInput     = $('domains-input');
const elBtnLookup = $('btn-lookup');
const elBtnNew    = $('btn-new');
const elBtnCopy   = $('btn-copy');
const elCounter   = $('counter');
const elProgress  = $('progress');
const elProgressBar = $('progress-bar');
const elBody      = $('results-body');
const elStats     = $('stats-section');
const elStatsBars = $('stats-bars');
const elPlaceholder = $('placeholder-row');

let results = [];
let resultMap = {}; // domain -> {index, data}
let domainOrder = [];
let total = 0;
let done = 0;

// ── Domain normalisation (mirrors backend normalizeDomain) ────────────────────
function normalizeDomain(s) {
  s = s.trim().toLowerCase();
  s = s.replace(/^https?:\/\//i, '');   // strip scheme
  s = s.replace(/[/?#].*$/, '');         // strip path / query / fragment
  s = s.replace(/:\d+$/, '');            // strip port
  s = s.replace(/^www\./, '');           // strip www.
  return s;
}

// ── Input counter ─────────────────────────────────────────────────────────────
elInput.addEventListener('input', updateCounter);

function updateCounter() {
  const domains = parseDomains(elInput.value);
  if (domains.length === 0) {
    elCounter.hidden = true;
    return;
  }
  elCounter.hidden = false;
  elCounter.textContent = `${domains.length} domain${domains.length !== 1 ? 's' : ''}`;
}

function parseDomains(raw) {
  return raw.split('\n')
    .map(s => normalizeDomain(s))
    .filter(s => s.length > 0 && s.includes('.'));
}

// ── Lookup ────────────────────────────────────────────────────────────────────
elBtnLookup.addEventListener('click', startLookup);
elBtnNew.addEventListener('click', resetUI);
elBtnCopy.addEventListener('click', copyCSV);

function startLookup() {
  const domains = parseDomains(elInput.value);
  if (domains.length === 0) return;

  results = [];
  resultMap = {};
  domainOrder = [...domains];
  total = domains.length;
  done = 0;

  elPlaceholder && elPlaceholder.remove();
  elBody.innerHTML = '';
  elStatsBars.innerHTML = '';
  elStats.hidden = true;
  elBtnCopy.hidden = true;
  elBtnNew.hidden = true;
  elBtnLookup.disabled = true;

  // Pre-render rows in input order with loading placeholders
  for (const domain of domainOrder) {
    prependLoadingRow(domain);
  }

  elProgress.hidden = false;
  elProgressBar.style.width = '0%';
  elCounter.hidden = false;
  elCounter.textContent = `0 / ${total}`;

  fetch('/api/lookup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ domains })
  }).then(resp => {
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';

    function pump() {
      reader.read().then(({ done: streamDone, value }) => {
        if (streamDone) { finishLookup(); return; }
        buf += decoder.decode(value, { stream: true });
        const parts = buf.split('\n\n');
        buf = parts.pop();
        for (const part of parts) {
          const line = part.trim();
          if (line.startsWith('data: ')) handleEvent(line.slice(6));
        }
        pump();
      }).catch(err => { console.error('SSE read error', err); finishLookup(); });
    }
    pump();
  }).catch(err => {
    console.error('Fetch error', err);
    elBtnLookup.disabled = false;
  });
}

function handleEvent(json) {
  let ev;
  try { ev = JSON.parse(json); } catch { return; }

  if (ev.type === 'done') {
    renderStats(ev.stats, ev.total);
    return;
  }

  results.push(ev);
  resultMap[ev.domain] = ev;
  done++;
  fillRow(ev);

  const pct = Math.round((done / total) * 100);
  elProgressBar.style.width = pct + '%';
  elCounter.textContent = `${done} / ${total}`;
}

// ── Row rendering ─────────────────────────────────────────────────────────────
function prependLoadingRow(domain) {
  const tr = document.createElement('tr');
  tr.id = 'row-' + CSS.escape(domain);

  const tdDomain = document.createElement('td');
  tdDomain.textContent = domain;
  tr.appendChild(tdDomain);

  const tdLoading = document.createElement('td');
  tdLoading.colSpan = 6;
  tdLoading.className = 'cell-loading';
  tdLoading.textContent = '…';
  tr.appendChild(tdLoading);

  elBody.appendChild(tr);
}

function fillRow(r) {
  const tr = document.getElementById('row-' + CSS.escape(r.domain));
  if (!tr) return;

  // Remove loading placeholder cells
  while (tr.cells.length > 1) tr.deleteCell(1);

  // Registrar
  const tdReg = document.createElement('td');
  if (r.source === 'error') {
    tdReg.className = 'cell-error';
    tdReg.textContent = r.error || 'lookup failed';
  } else {
    tdReg.className = 'cell-registrar';
    tdReg.textContent = r.registrar || '—';
  }
  tr.appendChild(tdReg);

  // Source badge
  const tdSrc = document.createElement('td');
  const badge = document.createElement('span');
  badge.className = `badge badge-${r.source}`;
  badge.textContent = r.source;
  tdSrc.appendChild(badge);
  tr.appendChild(tdSrc);

  // HTTP check columns
  const h = r.http;
  if (h && !h.http_error) {
    tr.appendChild(makeCodeCell(h.direct_code));
    tr.appendChild(makeCodeCell(h.bot_code));
    tr.appendChild(makeCodeCell(h.ref_code));
    tr.appendChild(makeSimCell(h.sim_bot_dir, h.sim_ref_dir));

    const sims = [h.sim_bot_dir, h.sim_ref_dir].filter(v => v != null);
    if (sims.some(v => v < 70)) tr.classList.add('row-cloak');
  } else {
    const errMsg = h ? h.http_error : '—';
    tr.appendChild(makeEmptyCell(4, errMsg));
  }
}

function makeCodeCell(code) {
  const td = document.createElement('td');
  if (!code) {
    td.innerHTML = '<span class="badge-code code-err">—</span>';
    return td;
  }
  const badge = document.createElement('span');
  badge.className = 'badge-code ' + codeClass(code);
  badge.textContent = code;
  td.appendChild(badge);
  return td;
}

function codeClass(code) {
  if (code >= 200 && code < 300) return 'code-2xx';
  if (code >= 300 && code < 400) return 'code-3xx';
  if (code >= 400)               return 'code-4xx';
  return 'code-err';
}

function makeSimCell(simBot, simRef) {
  const td = document.createElement('td');
  td.className = 'cell-sim';

  function simSpan(val, label) {
    const s = document.createElement('span');
    if (val == null) {
      s.className = 'sim-val sim-na';
      s.textContent = '—';
    } else {
      s.className = 'sim-val ' + simClass(val);
      s.textContent = val + '%';
    }
    s.title = label;
    return s;
  }

  td.appendChild(simSpan(simBot, 'Googlebot vs Direct'));
  const sep = document.createElement('span');
  sep.className = 'sim-sep';
  sep.textContent = ' · ';
  td.appendChild(sep);
  td.appendChild(simSpan(simRef, 'Google-Ref vs Direct'));
  return td;
}

function simClass(pct) {
  if (pct >= 90) return 'sim-high';
  if (pct >= 70) return 'sim-mid';
  return 'sim-low';
}

function makeEmptyCell(colspan, text) {
  const td = document.createElement('td');
  td.colSpan = colspan;
  td.className = 'cell-http-err';
  td.textContent = text || '—';
  return td;
}

function finishLookup() {
  elBtnLookup.disabled = false;
  elBtnNew.hidden = false;
  if (results.length > 0) elBtnCopy.hidden = false;
  elProgressBar.style.width = '100%';
  elCounter.textContent = `${done} / ${total} — done`;
}

// ── Stats ─────────────────────────────────────────────────────────────────────
function renderStats(stats, total) {
  if (!stats || Object.keys(stats).length === 0) return;

  const sorted = Object.entries(stats).sort((a, b) => b[1] - a[1]);

  elStatsBars.innerHTML = '';
  for (const [name, count] of sorted) {
    const pct = total > 0 ? Math.round((count / total) * 100) : 0;

    const row = document.createElement('div');
    row.className = 'stat-row';

    const nameEl = document.createElement('div');
    nameEl.className = 'stat-name';
    nameEl.title = name;
    nameEl.textContent = name;

    const barBg = document.createElement('div');
    barBg.className = 'stat-bar-bg';
    const fill = document.createElement('div');
    fill.className = 'stat-bar-fill';
    fill.style.width = '0%';
    barBg.appendChild(fill);

    const pctEl = document.createElement('div');
    pctEl.className = 'stat-pct';
    pctEl.textContent = `${pct}%`;

    row.appendChild(nameEl);
    row.appendChild(barBg);
    row.appendChild(pctEl);
    elStatsBars.appendChild(row);

    requestAnimationFrame(() => {
      requestAnimationFrame(() => { fill.style.width = pct + '%'; });
    });
  }

  elStats.hidden = false;
}

// ── CSV copy ──────────────────────────────────────────────────────────────────
function copyCSV() {
  const lines = ['domain,registrar,source,direct_code,bot_code,ref_code,sim_bot,sim_ref'];
  for (const r of results) {
    const reg = r.source === 'error' ? '' : (r.registrar || '');
    const h = r.http || {};
    lines.push([
      r.domain,
      csvEscape(reg),
      r.source,
      h.direct_code || '',
      h.bot_code || '',
      h.ref_code || '',
      h.sim_bot_dir != null ? h.sim_bot_dir : '',
      h.sim_ref_dir != null ? h.sim_ref_dir : '',
    ].join(','));
  }
  navigator.clipboard.writeText(lines.join('\n')).then(() => {
    const orig = elBtnCopy.textContent;
    elBtnCopy.textContent = 'Copied!';
    setTimeout(() => { elBtnCopy.textContent = orig; }, 1500);
  });
}

function csvEscape(s) {
  if (s.includes(',') || s.includes('"') || s.includes('\n')) {
    return '"' + s.replace(/"/g, '""') + '"';
  }
  return s;
}

// ── Reset ─────────────────────────────────────────────────────────────────────
function resetUI() {
  elInput.value = '';
  elBtnNew.hidden = true;
  elBtnCopy.hidden = true;
  elBtnLookup.disabled = false;
  elProgress.hidden = true;
  elProgressBar.style.width = '0%';
  elCounter.hidden = true;
  elStats.hidden = true;
  elStatsBars.innerHTML = '';
  elBody.innerHTML = '<tr id="placeholder-row"><td colspan="7" class="placeholder">Enter domains and click Lookup</td></tr>';
  results = [];
  resultMap = {};
  domainOrder = [];
  total = 0;
  done = 0;
}

updateCounter();
