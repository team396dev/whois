'use strict';

const $ = id => document.getElementById(id);

const elInput    = $('domains-input');
const elBtnLookup = $('btn-lookup');
const elBtnNew   = $('btn-new');
const elBtnCopy  = $('btn-copy');
const elCounter  = $('counter');
const elProgress = $('progress');
const elProgressBar = $('progress-bar');
const elBody     = $('results-body');
const elStats    = $('stats-section');
const elStatsBars = $('stats-bars');
const elPlaceholder = $('placeholder-row');

let results = [];
let total = 0;
let done = 0;
let activeSource = null;

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
    .map(s => s.trim().toLowerCase())
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
  total = domains.length;
  done = 0;

  elPlaceholder && elPlaceholder.remove();
  elBody.innerHTML = '';
  elStatsBars.innerHTML = '';
  elStats.hidden = true;
  elBtnCopy.hidden = true;
  elBtnNew.hidden = true;
  elBtnLookup.disabled = true;

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
        if (streamDone) {
          finishLookup();
          return;
        }
        buf += decoder.decode(value, { stream: true });
        const parts = buf.split('\n\n');
        buf = parts.pop(); // incomplete chunk
        for (const part of parts) {
          const line = part.trim();
          if (line.startsWith('data: ')) {
            handleEvent(line.slice(6));
          }
        }
        pump();
      }).catch(err => {
        console.error('SSE read error', err);
        finishLookup();
      });
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

  // individual domain result
  results.push(ev);
  done++;
  appendRow(ev);

  const pct = Math.round((done / total) * 100);
  elProgressBar.style.width = pct + '%';
  elCounter.textContent = `${done} / ${total}`;
}

function appendRow(r) {
  const tr = document.createElement('tr');

  const tdDomain = document.createElement('td');
  tdDomain.textContent = r.domain;
  tr.appendChild(tdDomain);

  const tdReg = document.createElement('td');
  if (r.source === 'error') {
    tdReg.className = 'cell-error';
    tdReg.textContent = r.error || 'lookup failed';
  } else {
    tdReg.className = 'cell-registrar';
    tdReg.textContent = r.registrar || '—';
  }
  tr.appendChild(tdReg);

  const tdSrc = document.createElement('td');
  const badge = document.createElement('span');
  badge.className = `badge badge-${r.source}`;
  badge.textContent = r.source;
  tdSrc.appendChild(badge);
  tr.appendChild(tdSrc);

  elBody.appendChild(tr);
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

    // Animate bar
    requestAnimationFrame(() => {
      requestAnimationFrame(() => { fill.style.width = pct + '%'; });
    });
  }

  elStats.hidden = false;
}

// ── CSV copy ──────────────────────────────────────────────────────────────────
function copyCSV() {
  const lines = ['domain,registrar,source'];
  for (const r of results) {
    const reg = r.source === 'error' ? '' : (r.registrar || '');
    lines.push(`${r.domain},${csvEscape(reg)},${r.source}`);
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
  elBody.innerHTML = '<tr id="placeholder-row"><td colspan="3" class="placeholder">Enter domains and click Lookup</td></tr>';
  results = [];
  total = 0;
  done = 0;
}

updateCounter();
