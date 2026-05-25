'use strict';

const $ = id => document.getElementById(id);

// ── DOM refs ──────────────────────────────────────────────────────────────────
const elInput       = $('domains-input');
const elBtnLookup   = $('btn-lookup');
const elBtnNew      = $('btn-new');
const elBtnCopy     = $('btn-copy');
const elCounter     = $('counter');
const elProgress    = $('progress');
const elProgressBar = $('progress-bar');
const elBody        = $('results-body');
const elHeaderRow   = $('results-header-row');
const elStats       = $('stats-section');
const elStatsBars   = $('stats-bars');
const elFileDrop    = $('file-drop');
const elFileInput   = $('file-input');
const elFileLabel   = $('file-label');
const elCanonTip    = $('canon-tooltip');
const elCcPopup     = $('cc-popup');
const elCcPopupClose = $('cc-popup-close');
const elCcPopupTitle = $('cc-popup-title');
const elCcPopupBody  = $('cc-popup-body');

// ── State ─────────────────────────────────────────────────────────────────────
let terms       = [];       // loaded from .txt file
let results     = [];
let resultMap   = {};
let rowElements = new Map();
let domainOrder = [];
let total = 0;
let done  = 0;

// ── Domain normalisation ──────────────────────────────────────────────────────
function normalizeDomain(s) {
  s = s.trim().toLowerCase();
  s = s.replace(/^https?:\/\//i, '');
  s = s.replace(/[/?#].*$/, '');
  s = s.replace(/:\d+$/, '');
  s = s.replace(/^www\./, '');
  return s;
}

function parseDomains(raw) {
  return raw.split('\n').map(normalizeDomain).filter(s => s.length > 0 && s.includes('.'));
}

// ── Terms file ────────────────────────────────────────────────────────────────
elFileInput.addEventListener('change', () => loadTermFile(elFileInput.files[0]));
elFileDrop.addEventListener('dragover',  e => { e.preventDefault(); elFileDrop.classList.add('drag-over'); });
elFileDrop.addEventListener('dragleave', () => elFileDrop.classList.remove('drag-over'));
elFileDrop.addEventListener('drop', e => {
  e.preventDefault();
  elFileDrop.classList.remove('drag-over');
  const f = e.dataTransfer.files[0];
  if (f) loadTermFile(f);
});

function loadTermFile(file) {
  if (!file) return;
  const reader = new FileReader();
  reader.onload = e => {
    terms = e.target.result.split('\n').map(s => s.trim()).filter(s => s.length > 0);
    elFileLabel.textContent = `${file.name} — ${terms.length} term${terms.length !== 1 ? 's' : ''}`;
    elFileDrop.classList.add('has-file');
  };
  reader.readAsText(file);
}

// ── Counter ───────────────────────────────────────────────────────────────────
elInput.addEventListener('input', updateCounter);

function updateCounter() {
  const d = parseDomains(elInput.value);
  if (d.length === 0) { elCounter.hidden = true; return; }
  elCounter.hidden = false;
  elCounter.textContent = `${d.length} domain${d.length !== 1 ? 's' : ''}`;
}

// ── Lookup ────────────────────────────────────────────────────────────────────
elBtnLookup.addEventListener('click', startLookup);
elBtnNew.addEventListener('click', resetUI);
elBtnCopy.addEventListener('click', copyTSV);

function startLookup() {
  const domains = parseDomains(elInput.value);
  if (domains.length === 0) return;

  results = []; resultMap = {}; rowElements = new Map();
  domainOrder = [...domains]; total = domains.length; done = 0;

  $('placeholder-row') && $('placeholder-row').remove();
  elBody.innerHTML = '';
  elStatsBars.innerHTML = '';
  elStats.hidden = true;
  elBtnCopy.hidden = true;
  elBtnNew.hidden = true;
  elBtnLookup.disabled = true;

  // Build table header — add content columns if terms are loaded
  buildHeader();

  for (const domain of domainOrder) addLoadingRow(domain);

  elProgress.hidden = false;
  elProgressBar.style.width = '0%';
  elCounter.hidden = false;
  elCounter.textContent = `0 / ${total}`;

  const body = { domains };
  if (terms.length > 0) body.terms = terms;

  fetch('/api/lookup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(resp => {
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    function pump() {
      reader.read().then(({ done: sd, value }) => {
        if (sd) { finishLookup(); return; }
        buf += decoder.decode(value, { stream: true });
        const parts = buf.split('\n\n'); buf = parts.pop();
        for (const part of parts) {
          const line = part.trim();
          if (line.startsWith('data: ')) handleEvent(line.slice(6));
        }
        pump();
      }).catch(err => { console.error('SSE error', err); finishLookup(); });
    }
    pump();
  }).catch(err => { console.error('Fetch error', err); elBtnLookup.disabled = false; });
}

function handleEvent(json) {
  let ev;
  try { ev = JSON.parse(json); } catch { return; }
  if (ev.type === 'done') { renderStats(ev.stats, ev.total); return; }
  results.push(ev);
  resultMap[ev.domain] = ev;
  done++;
  fillRow(ev);
  const pct = Math.round((done / total) * 100);
  elProgressBar.style.width = pct + '%';
  elCounter.textContent = `${done} / ${total}`;
}

// ── Table header ──────────────────────────────────────────────────────────────
function buildHeader() {
  // Remove content-check th elements from previous run
  elHeaderRow.querySelectorAll('.th-content').forEach(el => el.remove());

  if (terms.length > 0) {
    const labels = [
      ['Direct', 'Content matches — no UA, no Referer'],
      ['Bot',    'Content matches — Googlebot UA'],
      ['Ref',    'Content matches — Chrome UA + Referer: google.com'],
    ];
    for (const [label, title] of labels) {
      const th = document.createElement('th');
      th.className = 'th-content';
      th.title = title;
      th.textContent = label + ' ✦';
      elHeaderRow.appendChild(th);
    }
  }
}

// ── Loading row ───────────────────────────────────────────────────────────────
function addLoadingRow(domain) {
  const tr = document.createElement('tr');
  rowElements.set(domain, tr);
  const td0 = document.createElement('td'); td0.textContent = domain; tr.appendChild(td0);
  const tdL = document.createElement('td');
  tdL.colSpan = 7 + (terms.length > 0 ? 3 : 0);
  tdL.className = 'cell-loading'; tdL.textContent = '…';
  tr.appendChild(tdL);
  elBody.appendChild(tr);
}

// ── Fill row ──────────────────────────────────────────────────────────────────
function fillRow(r) {
  const tr = rowElements.get(r.domain);
  if (!tr) return;
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

  // HTTP + canonical
  const h = r.http;
  if (h && !h.http_error) {
    tr.appendChild(makeCodeCell(h.direct_code));
    tr.appendChild(makeCodeCell(h.bot_code));
    tr.appendChild(makeCodeCell(h.ref_code));
    tr.appendChild(makeSimCell(h.sim_bot_dir, h.sim_ref_dir));
    tr.appendChild(makeCanonCell(h.canon));
    const sims = [h.sim_bot_dir, h.sim_ref_dir].filter(v => v != null);
    if (sims.some(v => v < 70)) tr.classList.add('row-cloak');
  } else {
    tr.appendChild(makeEmptyCell(5, h ? h.http_error : '—'));
  }

  // Content check columns (only if terms were sent)
  if (terms.length > 0) {
    const c = r.content;
    tr.appendChild(makeContentCell(c ? c.direct : null, r.domain, 'Direct'));
    tr.appendChild(makeContentCell(c ? c.bot    : null, r.domain, 'Bot'));
    tr.appendChild(makeContentCell(c ? c.ref    : null, r.domain, 'Ref'));
  }
}

// ── HTTP code cell ────────────────────────────────────────────────────────────
function makeCodeCell(code) {
  const td = document.createElement('td');
  if (!code) { td.innerHTML = '<span class="badge-code code-err">—</span>'; return td; }
  const b = document.createElement('span');
  b.className = 'badge-code ' + codeClass(code);
  b.textContent = code;
  td.appendChild(b);
  return td;
}

function codeClass(code) {
  if (code >= 200 && code < 300) return 'code-2xx';
  if (code >= 300 && code < 400) return 'code-3xx';
  if (code >= 400)               return 'code-4xx';
  return 'code-err';
}

// ── Similarity cell ───────────────────────────────────────────────────────────
function makeSimCell(simBot, simRef) {
  const td = document.createElement('td');
  td.className = 'cell-sim';
  function simSpan(val, label) {
    const s = document.createElement('span');
    if (val == null) { s.className = 'sim-val sim-na'; s.textContent = '—'; }
    else { s.className = 'sim-val ' + simClass(val); s.textContent = val + '%'; }
    s.title = label;
    return s;
  }
  td.appendChild(simSpan(simBot, 'Googlebot vs Direct'));
  const sep = document.createElement('span'); sep.className = 'sim-sep'; sep.textContent = ' · ';
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
  td.colSpan = colspan; td.className = 'cell-http-err'; td.textContent = text || '—';
  return td;
}

// ── Canonical cell ────────────────────────────────────────────────────────────
function makeCanonCell(canon) {
  const td = document.createElement('td');
  if (!canon) {
    const s = document.createElement('span');
    s.className = 'canon-icon canon-none'; s.textContent = '—';
    td.appendChild(s); return td;
  }
  const rel = canon.relation || 'none';
  const cls  = rel === 'match' ? 'canon-match' : rel === 'other_site' ? 'canon-other'
             : (rel === 'subfolder' || rel === 'subdomain') ? 'canon-sub' : 'canon-none';
  const char = rel === 'match' ? '✓' : rel === 'other_site' ? '✕' : rel === 'none' ? '—' : '~';

  const s = document.createElement('span');
  s.className = 'canon-icon ' + cls; s.textContent = char;
  s.addEventListener('mouseenter', e => showCanonTooltip(e, canon));
  s.addEventListener('mousemove',  positionCanonTip);
  s.addEventListener('mouseleave', () => { elCanonTip.hidden = true; });
  td.appendChild(s);
  return td;
}

function relLabel(rel) {
  return { match: 'Match', subfolder: 'Subfolder', subdomain: 'Subdomain', other_site: 'Other site', none: 'None' }[rel] || rel;
}
function relCls(rel) {
  return rel === 'match' ? 'ct-rel-match' : rel === 'other_site' ? 'ct-rel-other' : 'ct-rel-sub';
}

function showCanonTooltip(e, canon) {
  let html = '';
  if (canon.url) {
    html += `<div class="ct-row">
      <span class="ct-label">Canonical</span>
      <span class="ct-url">${esc(canon.url)}</span>
      <span class="ct-rel ${relCls(canon.relation)}">${relLabel(canon.relation)}</span>
    </div>`;
  } else {
    html += `<div class="ct-row"><span class="ct-label">Canonical</span>
      <span class="ct-url" style="color:var(--text-muted)">not found</span></div>`;
  }
  if (canon.alts && canon.alts.length > 0) {
    html += `<hr class="ct-divider"><div class="ct-row">
      <span class="ct-label">Alternates (${canon.alts.length})</span></div>`;
    for (const alt of canon.alts) {
      html += `<div class="ct-row">
        <span class="ct-url">${esc(alt.href)}</span>
        <span style="display:flex;gap:6px;align-items:center;flex-wrap:wrap">
          ${alt.hreflang ? `<span class="ct-lang">${esc(alt.hreflang)}</span>` : ''}
          <span class="ct-rel ${relCls(alt.relation)}">${relLabel(alt.relation)}</span>
        </span>
      </div>`;
    }
  }
  elCanonTip.innerHTML = html;
  elCanonTip.hidden = false;
  positionCanonTip(e);
}

function positionCanonTip(e) {
  const m = 12, vpW = window.innerWidth, vpH = window.innerHeight;
  const w = elCanonTip.offsetWidth || 300, h = elCanonTip.offsetHeight || 120;
  let x = e.clientX + m, y = e.clientY + m;
  if (x + w > vpW - m) x = e.clientX - w - m;
  if (y + h > vpH - m) y = e.clientY - h - m;
  elCanonTip.style.left = x + 'px';
  elCanonTip.style.top  = y + 'px';
}

// ── Content count cell ────────────────────────────────────────────────────────
function makeContentCell(res, domain, label) {
  const td = document.createElement('td');
  if (!res) { const s = document.createElement('span'); s.className = 'cc-err'; s.textContent = '—'; td.appendChild(s); return td; }
  if (res.error) { const s = document.createElement('span'); s.className = 'cc-err'; s.textContent = res.error; td.appendChild(s); return td; }

  const btn = document.createElement('button');
  btn.className = 'cc-count ' + (res.total > 0 ? 'cc-count-pos' : 'cc-count-zero');
  btn.textContent = res.total;
  btn.addEventListener('click', () => showCcBreakdown(domain, label, res));
  td.appendChild(btn);
  return td;
}

// ── Content breakdown popup ───────────────────────────────────────────────────
function showCcBreakdown(domain, label, res) {
  elCcPopupTitle.textContent = `${domain} — ${label}`;
  elCcPopupBody.innerHTML = '';
  const items = res.by_term || [];
  if (items.length === 0) {
    elCcPopupBody.innerHTML = '<span class="cc-err">No data</span>';
  } else {
    for (const tc of items) {
      const row = document.createElement('div'); row.className = 'cc-term-row';
      const nm = document.createElement('span'); nm.className = 'cc-term-name'; nm.textContent = tc.term;
      const ct = document.createElement('span');
      ct.className = 'cc-term-count' + (tc.count > 0 ? ' pos' : '');
      ct.textContent = tc.count;
      row.appendChild(nm); row.appendChild(ct);
      elCcPopupBody.appendChild(row);
    }
  }
  elCcPopup.hidden = false;
}

elCcPopupClose.addEventListener('click', () => { elCcPopup.hidden = true; });
elCcPopup.addEventListener('click', e => { if (e.target === elCcPopup) elCcPopup.hidden = true; });

// ── Finish / Stats ────────────────────────────────────────────────────────────
function finishLookup() {
  elBtnLookup.disabled = false;
  elBtnNew.hidden = false;
  if (results.length > 0) elBtnCopy.hidden = false;
  elProgressBar.style.width = '100%';
  elCounter.textContent = `${done} / ${total} — done`;
}

function renderStats(stats, total) {
  if (!stats || Object.keys(stats).length === 0) return;
  const sorted = Object.entries(stats).sort((a, b) => b[1] - a[1]);
  elStatsBars.innerHTML = '';
  for (const [name, count] of sorted) {
    const pct = total > 0 ? Math.round((count / total) * 100) : 0;
    const row = document.createElement('div'); row.className = 'stat-row';
    const nm = document.createElement('div'); nm.className = 'stat-name'; nm.title = name; nm.textContent = name;
    const bg = document.createElement('div'); bg.className = 'stat-bar-bg';
    const fill = document.createElement('div'); fill.className = 'stat-bar-fill'; fill.style.width = '0%';
    bg.appendChild(fill);
    const pct2 = document.createElement('div'); pct2.className = 'stat-pct'; pct2.textContent = `${pct}%`;
    row.appendChild(nm); row.appendChild(bg); row.appendChild(pct2);
    elStatsBars.appendChild(row);
    requestAnimationFrame(() => { requestAnimationFrame(() => { fill.style.width = pct + '%'; }); });
  }
  elStats.hidden = false;
}

// ── TSV copy ──────────────────────────────────────────────────────────────────
function copyTSV() {
  const T = '\t';
  const baseHead = ['domain','registrar','source','direct_code','bot_code','ref_code','sim_bot','sim_ref','canon_rel','canon_url'];
  const contHead = terms.length > 0 ? ['direct_terms','bot_terms','ref_terms'] : [];
  const lines = [baseHead.concat(contHead).join(T)];

  for (const domain of domainOrder) {
    const r = resultMap[domain]; if (!r) continue;
    const reg = r.source === 'error' ? '' : (r.registrar || '');
    const h = r.http || {}, c = h.canon || {};
    const base = [
      r.domain, reg, r.source,
      h.direct_code || '', h.bot_code || '', h.ref_code || '',
      h.sim_bot_dir != null ? h.sim_bot_dir : '',
      h.sim_ref_dir != null ? h.sim_ref_dir : '',
      c.relation || '', c.url || '',
    ];
    const cont = terms.length > 0 ? [
      r.content ? r.content.direct.total : '',
      r.content ? r.content.bot.total    : '',
      r.content ? r.content.ref.total    : '',
    ] : [];
    lines.push(base.concat(cont).join(T));
  }
  navigator.clipboard.writeText(lines.join('\n')).then(() => {
    const orig = elBtnCopy.textContent;
    elBtnCopy.textContent = 'Copied!';
    setTimeout(() => { elBtnCopy.textContent = orig; }, 1500);
  });
}

// ── Reset ─────────────────────────────────────────────────────────────────────
function resetUI() {
  elInput.value = '';
  elBtnNew.hidden = true; elBtnCopy.hidden = true;
  elBtnLookup.disabled = false;
  elProgress.hidden = true; elProgressBar.style.width = '0%';
  elCounter.hidden = true;
  elStats.hidden = true; elStatsBars.innerHTML = '';
  // Restore base header (remove content columns)
  elHeaderRow.querySelectorAll('.th-content').forEach(el => el.remove());
  elBody.innerHTML = '<tr id="placeholder-row"><td colspan="8" class="placeholder">Enter domains and click Lookup</td></tr>';
  results = []; resultMap = {}; rowElements = new Map(); domainOrder = []; total = 0; done = 0;
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

updateCounter();

// ── Telegram popup ────────────────────────────────────────────────────────────
(function () {
  const COOKIE = '369_tg_closed';
  if (document.cookie.split('; ').some(c => c.startsWith(COOKIE + '='))) return;
  const overlay = $('tg-popup');
  overlay.removeAttribute('hidden');
  function close() {
    overlay.setAttribute('hidden', '');
    const exp = new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toUTCString();
    document.cookie = COOKIE + '=1; expires=' + exp + '; path=/';
  }
  $('tg-popup-close').addEventListener('click', close);
  overlay.addEventListener('click', e => { if (e.target === overlay) close(); });
})();
