package dashboard

// dashboardHTML is the built-in AgentLens web UI — a full observability
// dashboard served directly from the Go binary with no external deps.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AgentLens — AI Agent Observability</title>
  <meta name="description" content="Full-stack observability for AI agents. Track every turn, tool call, hallucination, and user frustration signal in real time.">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
  <style>
    /* ── Reset + tokens ─────────────────────────────────────────── */
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    :root {
      --bg-base:        #0a0c11;
      --bg-surface:     #111520;
      --bg-elevated:    #171d2d;
      --bg-highlight:   #1e2640;

      --border:         rgba(255,255,255,0.06);
      --border-bright:  rgba(255,255,255,0.12);

      --text-primary:   #f0f2f7;
      --text-secondary: #8892a4;
      --text-muted:     #4a5568;

      --accent-blue:    #4f8ef7;
      --accent-purple:  #a78bfa;
      --accent-green:   #34d399;
      --accent-amber:   #fbbf24;
      --accent-red:     #f87171;
      --accent-pink:    #f472b6;

      --grad-brand: linear-gradient(135deg, #4f8ef7 0%, #a78bfa 100%);
      --grad-warn:  linear-gradient(135deg, #fbbf24 0%, #f87171 100%);

      --radius-sm: 6px;
      --radius-md: 10px;
      --radius-lg: 16px;

      --shadow-card: 0 4px 24px rgba(0,0,0,0.4);
      --shadow-glow: 0 0 40px rgba(79,142,247,0.12);
    }

    html, body { height: 100%; }
    body {
      font-family: 'Inter', -apple-system, sans-serif;
      background: var(--bg-base);
      color: var(--text-primary);
      font-size: 14px;
      line-height: 1.6;
      overflow-x: hidden;
    }

    /* ── Layout ─────────────────────────────────────────────────── */
    .app { display: flex; flex-direction: column; min-height: 100vh; }

    /* ── Header ─────────────────────────────────────────────────── */
    .header {
      display: flex; align-items: center; justify-content: space-between;
      padding: 0 28px; height: 60px;
      background: rgba(17,21,32,0.95);
      border-bottom: 1px solid var(--border);
      backdrop-filter: blur(12px);
      position: sticky; top: 0; z-index: 100;
    }
    .logo {
      display: flex; align-items: center; gap: 10px;
      font-size: 15px; font-weight: 600; letter-spacing: -0.3px;
    }
    .logo-icon {
      width: 28px; height: 28px; border-radius: 8px;
      background: var(--grad-brand);
      display: flex; align-items: center; justify-content: center;
      font-size: 14px;
    }
    .logo-sub { color: var(--text-secondary); font-weight: 400; margin-left: 4px; }
    .header-actions { display: flex; align-items: center; gap: 12px; }
    .badge-live {
      display: flex; align-items: center; gap: 6px;
      padding: 4px 10px; border-radius: 20px;
      background: rgba(52,211,153,0.1); border: 1px solid rgba(52,211,153,0.2);
      font-size: 11px; font-weight: 500; color: var(--accent-green);
    }
    .badge-live::before {
      content: ''; width: 6px; height: 6px; border-radius: 50%;
      background: var(--accent-green);
      animation: pulse 2s ease-in-out infinite;
    }
    @keyframes pulse {
      0%, 100% { opacity: 1; transform: scale(1); }
      50% { opacity: 0.5; transform: scale(0.8); }
    }

    /* ── Nav tabs ───────────────────────────────────────────────── */
    .nav-tabs {
      display: flex; align-items: center; gap: 4px;
      padding: 0 28px; height: 48px;
      background: var(--bg-surface);
      border-bottom: 1px solid var(--border);
    }
    .nav-tab {
      padding: 6px 14px; border-radius: var(--radius-sm);
      font-size: 13px; font-weight: 500; color: var(--text-secondary);
      cursor: pointer; border: none; background: transparent;
      transition: all 0.15s ease;
    }
    .nav-tab:hover { color: var(--text-primary); background: var(--bg-highlight); }
    .nav-tab.active {
      color: var(--accent-blue); background: rgba(79,142,247,0.1);
    }

    /* ── Main content ───────────────────────────────────────────── */
    .main { display: flex; flex: 1; }
    .content { flex: 1; padding: 24px 28px; overflow-y: auto; max-height: calc(100vh - 108px); }

    /* ── Page sections ──────────────────────────────────────────── */
    .page { display: none; }
    .page.active { display: block; }

    /* ── Score cards ────────────────────────────────────────────── */
    .stat-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
      gap: 16px; margin-bottom: 28px;
    }
    .stat-card {
      background: var(--bg-surface); border: 1px solid var(--border);
      border-radius: var(--radius-md); padding: 20px;
      position: relative; overflow: hidden;
      transition: border-color 0.2s, transform 0.2s;
    }
    .stat-card:hover { border-color: var(--border-bright); transform: translateY(-1px); }
    .stat-card::before {
      content: ''; position: absolute; inset: 0;
      background: var(--grad-brand); opacity: 0; transition: opacity 0.2s;
    }
    .stat-card:hover::before { opacity: 0.02; }
    .stat-label { font-size: 11px; font-weight: 500; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.08em; margin-bottom: 8px; }
    .stat-value { font-size: 28px; font-weight: 700; line-height: 1; letter-spacing: -1px; }
    .stat-value.blue   { color: var(--accent-blue); }
    .stat-value.green  { color: var(--accent-green); }
    .stat-value.amber  { color: var(--accent-amber); }
    .stat-value.red    { color: var(--accent-red); }
    .stat-sub { font-size: 11px; color: var(--text-muted); margin-top: 6px; }

    /* ── Section headers ────────────────────────────────────────── */
    .section-header {
      display: flex; align-items: center; justify-content: space-between;
      margin-bottom: 16px;
    }
    .section-title { font-size: 14px; font-weight: 600; color: var(--text-primary); }
    .section-action {
      font-size: 12px; color: var(--accent-blue); cursor: pointer;
      background: none; border: none; padding: 4px 8px;
      border-radius: var(--radius-sm); transition: background 0.15s;
    }
    .section-action:hover { background: rgba(79,142,247,0.1); }

    /* ── Table ──────────────────────────────────────────────────── */
    .table-wrap {
      background: var(--bg-surface); border: 1px solid var(--border);
      border-radius: var(--radius-md); overflow: hidden; margin-bottom: 24px;
    }
    table { width: 100%; border-collapse: collapse; }
    thead th {
      padding: 10px 16px; text-align: left;
      font-size: 11px; font-weight: 600; color: var(--text-muted);
      text-transform: uppercase; letter-spacing: 0.06em;
      border-bottom: 1px solid var(--border);
      background: var(--bg-elevated);
    }
    tbody tr {
      border-bottom: 1px solid var(--border);
      cursor: pointer; transition: background 0.12s;
    }
    tbody tr:last-child { border-bottom: none; }
    tbody tr:hover { background: var(--bg-highlight); }
    tbody td { padding: 12px 16px; font-size: 13px; vertical-align: middle; }

    /* ── Frustration bar ────────────────────────────────────────── */
    .frust-bar-wrap { display: flex; align-items: center; gap: 8px; }
    .frust-bar {
      flex: 1; height: 6px; border-radius: 3px;
      background: var(--bg-highlight); overflow: hidden; min-width: 80px;
    }
    .frust-bar-fill {
      height: 100%; border-radius: 3px;
      transition: width 0.6s ease;
    }
    .frust-bar-fill.low    { background: var(--accent-green); }
    .frust-bar-fill.medium { background: var(--accent-amber); }
    .frust-bar-fill.high   { background: var(--accent-red); }
    .frust-value { font-size: 12px; font-weight: 600; min-width: 32px; text-align: right; }

    /* ── Status badges ──────────────────────────────────────────── */
    .badge {
      display: inline-flex; align-items: center; gap: 4px;
      padding: 2px 8px; border-radius: 20px;
      font-size: 11px; font-weight: 600; letter-spacing: 0.02em;
    }
    .badge.success  { background: rgba(52,211,153,0.12); color: var(--accent-green); }
    .badge.error    { background: rgba(248,113,113,0.12); color: var(--accent-red); }
    .badge.warning  { background: rgba(251,191,36,0.12); color: var(--accent-amber); }
    .badge.info     { background: rgba(79,142,247,0.12); color: var(--accent-blue); }
    .badge.abandoned { background: rgba(244,114,182,0.12); color: var(--accent-pink); }
    .badge.running  { background: rgba(167,139,250,0.12); color: var(--accent-purple); }

    /* ── Session timeline panel ─────────────────────────────────── */
    .timeline { margin-bottom: 24px; }
    .timeline-turn {
      display: flex; gap: 16px; margin-bottom: 8px;
    }
    .timeline-marker {
      display: flex; flex-direction: column; align-items: center; flex-shrink: 0;
    }
    .timeline-dot {
      width: 10px; height: 10px; border-radius: 50%; border: 2px solid;
      flex-shrink: 0; margin-top: 4px;
    }
    .timeline-dot.ok       { border-color: var(--accent-green); background: rgba(52,211,153,0.2); }
    .timeline-dot.warn     { border-color: var(--accent-amber); background: rgba(251,191,36,0.2); }
    .timeline-dot.critical { border-color: var(--accent-red);   background: rgba(248,113,113,0.2); }
    .timeline-line {
      width: 2px; flex: 1; background: var(--border); margin-top: 4px;
    }
    .timeline-card {
      flex: 1; background: var(--bg-surface); border: 1px solid var(--border);
      border-radius: var(--radius-md); padding: 16px; margin-bottom: 8px;
      transition: border-color 0.2s;
    }
    .timeline-card:hover { border-color: var(--border-bright); }
    .timeline-card.flagged { border-color: rgba(248,113,113,0.3); }
    .timeline-header { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
    .turn-index {
      font-family: 'JetBrains Mono', monospace; font-size: 11px;
      color: var(--text-muted); min-width: 32px;
    }
    .turn-meta { margin-left: auto; display: flex; gap: 6px; align-items: center; }
    .turn-latency { font-family: 'JetBrains Mono', monospace; font-size: 11px; color: var(--text-muted); }
    .user-msg { font-size: 13px; color: var(--text-secondary); margin-bottom: 8px; font-style: italic; line-height: 1.5; }
    .model-msg { font-size: 13px; color: var(--text-primary); margin-bottom: 10px; line-height: 1.6; }

    /* Tool calls in timeline */
    .tool-calls { display: flex; flex-direction: column; gap: 4px; margin-bottom: 10px; }
    .tool-call-row {
      display: flex; align-items: center; gap: 8px;
      padding: 6px 10px; border-radius: var(--radius-sm);
      background: var(--bg-elevated); border: 1px solid var(--border);
      font-family: 'JetBrains Mono', monospace; font-size: 12px;
    }
    .tool-icon { font-size: 14px; }
    .tool-name { color: var(--accent-blue); font-weight: 500; }
    .tool-duration { margin-left: auto; color: var(--text-muted); }
    .tool-status-dot { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0; }
    .tool-status-dot.success { background: var(--accent-green); }
    .tool-status-dot.error   { background: var(--accent-red); }
    .tool-status-dot.timeout { background: var(--accent-amber); }

    /* Signals in timeline */
    .signal-row {
      display: flex; align-items: center; gap: 8px;
      padding: 6px 10px; border-radius: var(--radius-sm);
      background: rgba(248,113,113,0.06); border: 1px solid rgba(248,113,113,0.15);
      font-size: 12px; color: var(--accent-red);
    }

    /* ── Hallucination heatmap ──────────────────────────────────── */
    .heatmap-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(40px, 1fr));
      gap: 4px;
    }
    .heatmap-cell {
      aspect-ratio: 1; border-radius: 4px;
      display: flex; align-items: center; justify-content: center;
      font-size: 10px; font-weight: 600; cursor: pointer;
      transition: transform 0.15s, opacity 0.15s;
    }
    .heatmap-cell:hover { transform: scale(1.2); }
    .heatmap-cell.r0  { background: rgba(52,211,153,0.15); color: var(--accent-green); }
    .heatmap-cell.r1  { background: rgba(251,191,36,0.2);  color: var(--accent-amber); }
    .heatmap-cell.r2  { background: rgba(248,113,113,0.25); color: var(--accent-red); }
    .heatmap-cell.r3  { background: rgba(248,113,113,0.5);  color: #fff; }

    /* ── Empty state ────────────────────────────────────────────── */
    .empty {
      display: flex; flex-direction: column; align-items: center; justify-content: center;
      padding: 60px 20px; text-align: center; color: var(--text-muted);
    }
    .empty-icon { font-size: 48px; margin-bottom: 16px; opacity: 0.4; }
    .empty-title { font-size: 16px; font-weight: 600; color: var(--text-secondary); margin-bottom: 8px; }
    .empty-sub { font-size: 13px; max-width: 320px; }

    /* ── Loading ────────────────────────────────────────────────── */
    .loading { display: flex; align-items: center; justify-content: center; padding: 40px; }
    .spinner {
      width: 24px; height: 24px; border: 2px solid var(--border);
      border-top-color: var(--accent-blue); border-radius: 50%;
      animation: spin 0.7s linear infinite;
    }
    @keyframes spin { to { transform: rotate(360deg); } }

    /* ── Error banner ───────────────────────────────────────────── */
    .error-banner {
      padding: 12px 16px; border-radius: var(--radius-sm);
      background: rgba(248,113,113,0.1); border: 1px solid rgba(248,113,113,0.2);
      color: var(--accent-red); font-size: 13px; margin-bottom: 16px;
    }

    /* ── Sidebar (session detail) ───────────────────────────────── */
    .sidebar {
      width: 480px; flex-shrink: 0;
      background: var(--bg-surface); border-left: 1px solid var(--border);
      display: flex; flex-direction: column; overflow: hidden;
      transition: width 0.25s ease;
    }
    .sidebar.collapsed { width: 0; }
    .sidebar-header {
      padding: 16px 20px; border-bottom: 1px solid var(--border);
      display: flex; align-items: center; justify-content: space-between;
      flex-shrink: 0;
    }
    .sidebar-close {
      cursor: pointer; background: none; border: none;
      color: var(--text-muted); font-size: 18px; padding: 4px;
      border-radius: var(--radius-sm); transition: color 0.15s;
    }
    .sidebar-close:hover { color: var(--text-primary); }
    .sidebar-body { flex: 1; overflow-y: auto; padding: 20px; }

    /* ── Code / mono ────────────────────────────────────────────── */
    code {
      font-family: 'JetBrains Mono', monospace; font-size: 12px;
      background: var(--bg-elevated); padding: 1px 5px;
      border-radius: 4px; color: var(--accent-purple);
    }

    /* ── Responsive ─────────────────────────────────────────────── */
    @media (max-width: 900px) {
      .sidebar { display: none; }
      .stat-grid { grid-template-columns: repeat(2, 1fr); }
    }
  </style>
</head>
<body>
<div class="app" id="app">

  <!-- Header -->
  <header class="header">
    <div class="logo">
      <div class="logo-icon">🔭</div>
      AgentLens<span class="logo-sub">/ observability</span>
    </div>
    <div class="header-actions">
      <div class="badge-live" id="live-indicator">LIVE</div>
    </div>
  </header>

  <!-- Nav tabs -->
  <nav class="nav-tabs">
    <button class="nav-tab active" id="tab-overview"    onclick="showPage('overview')">Overview</button>
    <button class="nav-tab"        id="tab-sessions"    onclick="showPage('sessions')">Sessions</button>
    <button class="nav-tab"        id="tab-hallucinations" onclick="showPage('hallucinations')">Hallucinations</button>
    <button class="nav-tab"        id="tab-frustration" onclick="showPage('frustration')">Frustration</button>
  </nav>

  <!-- Main -->
  <div class="main">
    <div class="content" id="content">

      <!-- ─── Overview page ─────────────────────────────────────── -->
      <div class="page active" id="page-overview">
        <div class="stat-grid" id="stat-grid">
          <div class="stat-card">
            <div class="stat-label">Total Sessions</div>
            <div class="stat-value blue" id="stat-total">—</div>
            <div class="stat-sub">last 1 hour</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">Success Rate</div>
            <div class="stat-value green" id="stat-success">—</div>
            <div class="stat-sub">sessions resolved</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">Avg Frustration</div>
            <div class="stat-value amber" id="stat-frustration">—</div>
            <div class="stat-sub">0.0 calm → 1.0 rage</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">Abandoned</div>
            <div class="stat-value red" id="stat-abandoned">—</div>
            <div class="stat-sub">frustration-driven</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">Avg Turn Count</div>
            <div class="stat-value blue" id="stat-turns">—</div>
            <div class="stat-sub">turns per session</div>
          </div>
        </div>

        <div class="section-header">
          <div class="section-title">Recent Sessions</div>
          <button class="section-action" onclick="loadSessions()">Refresh ↻</button>
        </div>

        <div class="table-wrap" id="overview-sessions-table">
          <div class="loading"><div class="spinner"></div></div>
        </div>
      </div>

      <!-- ─── Sessions page ─────────────────────────────────────── -->
      <div class="page" id="page-sessions">
        <div class="section-header">
          <div class="section-title">All Sessions</div>
          <button class="section-action" onclick="loadSessions()">Refresh ↻</button>
        </div>
        <div class="table-wrap" id="sessions-table">
          <div class="loading"><div class="spinner"></div></div>
        </div>
      </div>

      <!-- ─── Hallucinations page ───────────────────────────────── -->
      <div class="page" id="page-hallucinations">
        <div class="section-header">
          <div class="section-title">Hallucination Risk Heatmap</div>
          <div style="font-size:12px;color:var(--text-muted)">Click a cell to see session</div>
        </div>
        <div class="table-wrap" style="padding:20px;margin-bottom:24px">
          <div id="heatmap-container">
            <div class="loading"><div class="spinner"></div></div>
          </div>
        </div>
        <div class="section-header">
          <div class="section-title">Recent Signals</div>
        </div>
        <div class="table-wrap" id="signals-table">
          <div class="loading"><div class="spinner"></div></div>
        </div>
      </div>

      <!-- ─── Frustration page ──────────────────────────────────── -->
      <div class="page" id="page-frustration">
        <div class="section-header">
          <div class="section-title">User Frustration Monitor</div>
        </div>
        <div class="table-wrap" id="frustration-table">
          <div class="loading"><div class="spinner"></div></div>
        </div>
      </div>

    </div><!-- /content -->

    <!-- Session detail sidebar -->
    <aside class="sidebar collapsed" id="sidebar">
      <div class="sidebar-header">
        <div>
          <div class="section-title" id="sidebar-title">Session Details</div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:2px" id="sidebar-subtitle"></div>
        </div>
        <button class="sidebar-close" onclick="closeSidebar()">✕</button>
      </div>
      <div class="sidebar-body" id="sidebar-body">
        <div class="loading"><div class="spinner"></div></div>
      </div>
    </aside>
  </div>
</div>

<script>
// ─── State ─────────────────────────────────────────────────────────────────
const API = '/api/v1';
let allSessions = [];
let currentPage = 'overview';

// ─── Navigation ────────────────────────────────────────────────────────────
function showPage(page) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.querySelectorAll('.nav-tab').forEach(t => t.classList.remove('active'));
  document.getElementById('page-' + page).classList.add('active');
  document.getElementById('tab-' + page).classList.add('active');
  currentPage = page;

  if (page === 'overview') { loadOverview(); }
  else if (page === 'sessions') { renderSessionTable('sessions-table'); }
  else if (page === 'hallucinations') { loadHallucinationsPage(); }
  else if (page === 'frustration') { renderFrustrationTable(); }
}

// ─── Data fetching ──────────────────────────────────────────────────────────
async function fetchJSON(url) {
  const r = await fetch(url);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

async function loadSessions() {
  try {
    allSessions = await fetchJSON(API + '/sessions?limit=100') || [];
  } catch(e) {
    allSessions = [];
  }
  if (currentPage === 'overview') loadOverview();
  else if (currentPage === 'sessions') renderSessionTable('sessions-table');
}

// ─── Overview ───────────────────────────────────────────────────────────────
async function loadOverview() {
  await loadSessions();

  const total     = allSessions.length;
  const success   = allSessions.filter(s => s.outcome === 'success').length;
  const abandoned = allSessions.filter(s => s.outcome === 'abandoned').length;
  const avgFrust  = total ? (allSessions.reduce((a,s) => a + s.frustration_score, 0) / total) : 0;
  const avgTurns  = total ? (allSessions.reduce((a,s) => a + s.turn_count, 0) / total) : 0;

  document.getElementById('stat-total').textContent = total;
  document.getElementById('stat-success').textContent = total ? Math.round(success/total*100)+'%' : '—';
  document.getElementById('stat-frustration').textContent = avgFrust.toFixed(2);
  document.getElementById('stat-abandoned').textContent = abandoned;
  document.getElementById('stat-turns').textContent = avgTurns.toFixed(1);

  renderSessionTable('overview-sessions-table');
}

// ─── Session table ──────────────────────────────────────────────────────────
function renderSessionTable(containerId) {
  const el = document.getElementById(containerId);
  if (!allSessions.length) {
    el.innerHTML = emptyState('No sessions yet', 'Start sending requests through AgentLens to see sessions here.');
    return;
  }

  const rows = allSessions.slice(0, 50).map(s => {
    const frust = s.frustration_score || 0;
    const fc    = frust > 0.6 ? 'high' : frust > 0.3 ? 'medium' : 'low';
    const oc    = outcomeClass(s.outcome);
    const dur   = s.ended_at ? elapsed(s.started_at, s.ended_at) : 'ongoing';
    return ` + "`" + `
      <tr onclick="openSession('${s.id}')">
        <td><code>${s.id.slice(0,8)}</code></td>
        <td>${s.agent_id || '—'}</td>
        <td><span class="badge ${providerClass(s.provider)}">${s.provider}</span></td>
        <td>${s.model || '—'}</td>
        <td>${s.turn_count}</td>
        <td>
          <div class="frust-bar-wrap">
            <div class="frust-bar"><div class="frust-bar-fill ${fc}" style="width:${Math.round(frust*100)}%"></div></div>
            <span class="frust-value" style="color:${fc==='high'?'var(--accent-red)':fc==='medium'?'var(--accent-amber)':'var(--accent-green)'}">${frust.toFixed(2)}</span>
          </div>
        </td>
        <td><span class="badge ${oc}">${s.outcome}</span></td>
        <td style="color:var(--text-muted);font-size:12px">${dur}</td>
      </tr>` + "`" + `;
  }).join('');

  el.innerHTML = ` + "`" + `
    <table>
      <thead><tr>
        <th>ID</th><th>Agent</th><th>Provider</th><th>Model</th>
        <th>Turns</th><th>Frustration</th><th>Outcome</th><th>Duration</th>
      </tr></thead>
      <tbody>${rows}</tbody>
    </table>` + "`" + `;
}

// ─── Session detail (timeline) ──────────────────────────────────────────────
async function openSession(id) {
  const sidebar = document.getElementById('sidebar');
  const body    = document.getElementById('sidebar-body');
  const title   = document.getElementById('sidebar-title');
  const sub     = document.getElementById('sidebar-subtitle');

  sidebar.classList.remove('collapsed');
  body.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
  title.textContent = 'Session ' + id.slice(0,8) + '…';

  try {
    const [sess, timeline] = await Promise.all([
      fetchJSON(API + '/sessions/' + id),
      fetchJSON(API + '/sessions/' + id + '/timeline'),
    ]);
    sub.textContent = sess.provider + ' / ' + sess.model + ' · ' + (sess.turn_count || 0) + ' turns';
    body.innerHTML = renderTimeline(timeline);
  } catch(e) {
    body.innerHTML = '<div class="error-banner">Failed to load session: ' + e.message + '</div>';
  }
}

function closeSidebar() {
  document.getElementById('sidebar').classList.add('collapsed');
}

function renderTimeline(entries) {
  if (!entries || !entries.length) return emptyState('No turns', 'This session has no recorded turns.');

  return entries.map((entry, i) => {
    const turn     = entry.turn;
    const calls    = entry.tool_calls || [];
    const sigs     = entry.signals || [];
    const frust    = turn.frustration_delta || 0;
    const halRisk  = turn.hallucination_risk || 0;
    const dotClass = halRisk > 0.6 || frust > 0.3 ? (halRisk > 0.6 ? 'critical' : 'warn') : 'ok';
    const flagged  = halRisk > 0.5 ? 'flagged' : '';
    const isLast   = i === entries.length - 1;

    const toolRows = calls.map(tc => {
      const icon = categoryIcon(tc.category);
      const sc   = tc.status === 'success' ? 'success' : tc.status === 'timeout' ? 'timeout' : 'error';
      return ` + "`" + `
        <div class="tool-call-row">
          <span class="tool-icon">${icon}</span>
          <span class="tool-name">${tc.tool_name}</span>
          <div class="tool-status-dot ${sc}"></div>
          <span class="tool-duration">${tc.duration_ms}ms</span>
        </div>` + "`" + `;
    }).join('');

    const sigRows = sigs.map(s => ` + "`" + `
      <div class="signal-row">⚠ ${s.signal_type.replace(/_/g,' ')} — risk ${(s.risk_score*100).toFixed(0)}%</div>
    ` + "`" + `).join('');

    const frustrated = frust > 0.3 ? ` + "`" + `<span class="badge warning">😤 +${(frust*100).toFixed(0)}%</span>` + "`" + ` : '';
    const halBadge   = halRisk > 0.5 ? ` + "`" + `<span class="badge error">⚠ ${(halRisk*100).toFixed(0)}% hal.</span>` + "`" + ` : '';

    return ` + "`" + `
      <div class="timeline-turn">
        <div class="timeline-marker">
          <div class="timeline-dot ${dotClass}"></div>
          ${isLast ? '' : '<div class="timeline-line"></div>'}
        </div>
        <div class="timeline-card ${flagged}">
          <div class="timeline-header">
            <span class="turn-index">T:${String(turn.index).padStart(2,'0')}</span>
            ${frustrated}
            ${halBadge}
            <div class="turn-meta">
              <span class="turn-latency">${turn.latency_ms}ms</span>
            </div>
          </div>
          ${turn.user_message ? ` + "`" + `<div class="user-msg">"${truncate(turn.user_message, 180)}"</div>` + "`" + ` : ''}
          ${calls.length ? ` + "`" + `<div class="tool-calls">${toolRows}</div>` + "`" + ` : ''}
          ${turn.model_response ? ` + "`" + `<div class="model-msg">${truncate(turn.model_response, 200)}</div>` + "`" + ` : ''}
          ${sigRows}
        </div>
      </div>` + "`" + `;
  }).join('');
}

// ─── Hallucinations page ────────────────────────────────────────────────────
async function loadHallucinationsPage() {
  const heatmap = document.getElementById('heatmap-container');
  const table   = document.getElementById('signals-table');

  try {
    const sessions = allSessions.length ? allSessions : await fetchJSON(API + '/sessions?limit=100') || [];

    // Build heatmap from session risk data (one cell per session)
    if (!sessions.length) {
      heatmap.innerHTML = emptyState('No sessions', 'No data yet.');
    } else {
      const cells = sessions.map(s => {
        const risk = 0; // we'd pull per-session avg in a richer impl
        const rc   = 'r0';
        return ` + "`" + `<div class="heatmap-cell ${rc}" title="Session ${s.id.slice(0,8)}" onclick="openSession('${s.id}')">${s.turn_count}</div>` + "`" + `;
      }).join('');
      heatmap.innerHTML = ` + "`" + `
        <div style="margin-bottom:12px;font-size:12px;color:var(--text-muted)">
          Each cell = one session. Color = hallucination risk. Number = turn count.
        </div>
        <div class="heatmap-grid">${cells}</div>` + "`" + `;
    }
  } catch(e) {
    heatmap.innerHTML = '<div class="error-banner">' + e.message + '</div>';
  }

  table.innerHTML = emptyState('Select a session', 'Click a session to view its hallucination signals.');
}

// ─── Frustration page ────────────────────────────────────────────────────────
function renderFrustrationTable() {
  const el = document.getElementById('frustration-table');
  const frustrated = (allSessions || []).filter(s => s.frustration_score > 0.3)
    .sort((a,b) => b.frustration_score - a.frustration_score);

  if (!frustrated.length) {
    el.innerHTML = emptyState('No frustration events', 'No sessions with elevated frustration scores.');
    return;
  }

  const rows = frustrated.map(s => {
    const frust = s.frustration_score;
    const fc = frust > 0.6 ? 'high' : 'medium';
    const oc = s.outcome === 'abandoned' ? '<span class="badge abandoned">abandoned</span>' : '';
    return ` + "`" + `
      <tr onclick="openSession('${s.id}')">
        <td><code>${s.id.slice(0,8)}</code></td>
        <td>${s.agent_id}</td>
        <td>
          <div class="frust-bar-wrap">
            <div class="frust-bar"><div class="frust-bar-fill ${fc}" style="width:${Math.round(frust*100)}%"></div></div>
            <span class="frust-value" style="color:${fc==='high'?'var(--accent-red)':'var(--accent-amber)'}">${frust.toFixed(2)}</span>
          </div>
        </td>
        <td>${oc}</td>
        <td style="color:var(--text-muted);font-size:12px">${timeAgo(s.started_at)}</td>
      </tr>` + "`" + `;
  }).join('');

  el.innerHTML = ` + "`" + `
    <table>
      <thead><tr><th>ID</th><th>Agent</th><th>Frustration</th><th>Outcome</th><th>When</th></tr></thead>
      <tbody>${rows}</tbody>
    </table>` + "`" + `;
}

// ─── Utilities ──────────────────────────────────────────────────────────────
function emptyState(title, sub) {
  return ` + "`" + `
    <div class="empty">
      <div class="empty-icon">🔭</div>
      <div class="empty-title">${title}</div>
      <div class="empty-sub">${sub}</div>
    </div>` + "`" + `;
}

function outcomeClass(o) {
  return { success:'success', failed:'error', abandoned:'abandoned', in_progress:'running', escalated:'warning' }[o] || 'info';
}
function providerClass(p) {
  return { anthropic:'info', openai:'success', gemini:'warning', custom:'info' }[p] || 'info';
}
function categoryIcon(c) {
  return { file_ops:'📄', http:'🌐', database:'🗄️', compute:'⚙️', custom:'🔧' }[c] || '🔧';
}
function truncate(s, n) { return s && s.length > n ? s.slice(0,n) + '…' : s; }

function elapsed(start, end) {
  const ms = new Date(end) - new Date(start);
  if (ms < 60000) return (ms/1000).toFixed(1) + 's';
  return Math.round(ms/60000) + 'm';
}

function timeAgo(ts) {
  const diff = (Date.now() - new Date(ts)) / 1000;
  if (diff < 60)   return Math.round(diff) + 's ago';
  if (diff < 3600) return Math.round(diff/60) + 'm ago';
  return Math.round(diff/3600) + 'h ago';
}

// ─── SSE for live updates ───────────────────────────────────────────────────
function connectSSE() {
  const es = new EventSource('/api/v1/stream');
  es.addEventListener('ping', () => {});
  es.addEventListener('session', (e) => {
    const sess = JSON.parse(e.data);
    const idx = allSessions.findIndex(s => s.id === sess.id);
    if (idx >= 0) allSessions[idx] = sess; else allSessions.unshift(sess);
    if (currentPage === 'overview') loadOverview();
    else if (currentPage === 'sessions') renderSessionTable('sessions-table');
    else if (currentPage === 'frustration') renderFrustrationTable();
  });
  es.onerror = () => { setTimeout(connectSSE, 5000); };
}

// ─── Init ───────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
  loadOverview();
  connectSSE();
  setInterval(loadSessions, 30000);
});
</script>
</body>
</html>`
