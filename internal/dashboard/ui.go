package dashboard

// dashboardHTML is the built-in AgentLens web UI — a full observability
// dashboard served directly from the Go binary with no external deps.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AgentLens — Agent Observability</title>
  <meta name="description" content="Full-stack observability for AI agents.">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
  <style>
    /* ── Reset + Modern Tokens (Vercel/Linear aesthetic) ── */
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    
    :root {
      --bg-base: #000000;
      --bg-surface: #0a0a0a;
      --bg-elevated: #111111;
      --bg-highlight: #1a1a1a;
      
      --border: #222222;
      --border-bright: #333333;
      
      --text-primary: #ededed;
      --text-secondary: #a1a1a1;
      --text-muted: #737373;
      
      --accent-blue: #0070f3;
      --accent-purple: #7928ca;
      --accent-green: #10b981;
      --accent-amber: #f59e0b;
      --accent-red: #ef4444;
      --accent-pink: #ff0080;
      
      --grad-brand: linear-gradient(135deg, #0070f3 0%, #ff0080 100%);
      --grad-warn: linear-gradient(135deg, #f59e0b 0%, #ef4444 100%);
      
      --radius-sm: 4px;
      --radius-md: 8px;
      --radius-lg: 12px;
      
      --shadow-card: 0 8px 30px rgba(0,0,0,0.4);
      --shadow-glow: 0 0 40px rgba(0, 112, 243, 0.15);
      
      --font-ui: 'Inter', -apple-system, sans-serif;
      --font-mono: 'JetBrains Mono', monospace;
    }

    html, body { height: 100%; }
    body {
      font-family: var(--font-ui);
      background: var(--bg-base);
      color: var(--text-primary);
      font-size: 14px;
      line-height: 1.6;
      overflow-x: hidden;
    }

    /* ── Layout ── */
    .app { display: flex; flex-direction: column; min-height: 100vh; }
    
    /* ── Header ── */
    .header {
      display: flex; align-items: center; justify-content: space-between;
      padding: 0 24px; height: 64px;
      background: rgba(0, 0, 0, 0.8);
      border-bottom: 1px solid var(--border);
      backdrop-filter: saturate(180%) blur(12px);
      position: sticky; top: 0; z-index: 100;
    }
    .logo {
      display: flex; align-items: center; gap: 12px;
      font-size: 16px; font-weight: 600; letter-spacing: -0.02em;
    }
    .logo-icon {
      width: 24px; height: 24px; border-radius: 6px;
      background: var(--grad-brand);
      display: flex; align-items: center; justify-content: center;
      font-size: 12px;
      box-shadow: var(--shadow-glow);
    }
    .logo-sub { color: var(--text-secondary); font-weight: 400; margin-left: 4px; }
    .badge-live {
      display: flex; align-items: center; gap: 6px;
      padding: 4px 10px; border-radius: 20px;
      background: rgba(16, 185, 129, 0.1); border: 1px solid rgba(16, 185, 129, 0.2);
      font-size: 12px; font-weight: 500; color: var(--accent-green);
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

    /* ── Nav tabs ── */
    .nav-tabs {
      display: flex; align-items: center; gap: 8px;
      padding: 0 24px; height: 56px;
      background: var(--bg-surface);
      border-bottom: 1px solid var(--border);
    }
    .nav-tab {
      padding: 8px 16px; border-radius: var(--radius-sm);
      font-size: 13px; font-weight: 500; color: var(--text-secondary);
      cursor: pointer; border: none; background: transparent;
      transition: all 0.2s ease;
    }
    .nav-tab:hover { color: var(--text-primary); background: var(--bg-highlight); }
    .nav-tab.active {
      color: var(--text-primary); background: var(--bg-elevated);
      border: 1px solid var(--border-bright);
    }

    /* ── Main content ── */
    .main { display: flex; flex: 1; }
    .content { flex: 1; padding: 32px 24px; overflow-y: auto; max-height: calc(100vh - 120px); }
    
    .page { display: none; animation: fadeIn 0.3s ease; }
    .page.active { display: block; }
    @keyframes fadeIn { from { opacity: 0; transform: translateY(5px); } to { opacity: 1; transform: translateY(0); } }

    /* ── Score cards ── */
    .stat-grid {
      display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
      gap: 16px; margin-bottom: 32px;
    }
    .stat-card {
      background: var(--bg-surface); border: 1px solid var(--border);
      border-radius: var(--radius-md); padding: 24px;
      position: relative; overflow: hidden;
      transition: border-color 0.2s, transform 0.2s;
    }
    .stat-card:hover { border-color: var(--border-bright); transform: translateY(-2px); box-shadow: var(--shadow-card); }
    .stat-card::before {
      content: ''; position: absolute; inset: 0;
      background: var(--grad-brand); opacity: 0; transition: opacity 0.3s;
    }
    .stat-card:hover::before { opacity: 0.03; }
    .stat-label { font-size: 12px; font-weight: 500; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 8px; }
    .stat-value { font-size: 32px; font-weight: 600; line-height: 1; letter-spacing: -0.04em; }
    .stat-value.blue { color: var(--accent-blue); }
    .stat-value.green { color: var(--accent-green); }
    .stat-value.amber { color: var(--accent-amber); }
    .stat-value.red { color: var(--accent-red); }
    .stat-sub { font-size: 12px; color: var(--text-muted); margin-top: 8px; }

    /* ── Section headers ── */
    .section-header {
      display: flex; align-items: center; justify-content: space-between;
      margin-bottom: 20px;
    }
    .section-title { font-size: 16px; font-weight: 600; color: var(--text-primary); letter-spacing: -0.01em; }
    
    /* ── Table ── */
    .table-wrap {
      background: var(--bg-surface); border: 1px solid var(--border);
      border-radius: var(--radius-md); overflow: hidden; margin-bottom: 32px;
    }
    table { width: 100%; border-collapse: collapse; text-align: left; }
    thead th {
      padding: 12px 20px; font-size: 12px; font-weight: 500; color: var(--text-muted);
      border-bottom: 1px solid var(--border); background: var(--bg-base);
    }
    tbody tr {
      border-bottom: 1px solid var(--border); cursor: pointer; transition: background 0.15s;
    }
    tbody tr:last-child { border-bottom: none; }
    tbody tr:hover { background: var(--bg-highlight); }
    tbody td { padding: 14px 20px; font-size: 13px; vertical-align: middle; }
    code { font-family: var(--font-mono); font-size: 12px; background: var(--bg-elevated); padding: 3px 6px; border-radius: var(--radius-sm); border: 1px solid var(--border); color: var(--text-primary); }

    /* ── Badges ── */
    .badge {
      display: inline-flex; align-items: center; padding: 3px 8px;
      border-radius: 4px; font-size: 11px; font-weight: 500; font-family: var(--font-mono);
      text-transform: uppercase; border: 1px solid transparent;
    }
    .badge.success { background: rgba(16,185,129,0.1); color: var(--accent-green); border-color: rgba(16,185,129,0.2); }
    .badge.error { background: rgba(239,68,68,0.1); color: var(--accent-red); border-color: rgba(239,68,68,0.2); }
    .badge.warning { background: rgba(245,158,11,0.1); color: var(--accent-amber); border-color: rgba(245,158,11,0.2); }
    .badge.info { background: rgba(0,112,243,0.1); color: var(--accent-blue); border-color: rgba(0,112,243,0.2); }
    .badge.neutral { background: var(--bg-elevated); color: var(--text-secondary); border-color: var(--border); }

    /* ── Frustration bar ── */
    .frust-bar-wrap { display: flex; align-items: center; gap: 10px; }
    .frust-bar { flex: 1; height: 4px; border-radius: 2px; background: var(--bg-elevated); overflow: hidden; min-width: 100px; }
    .frust-bar-fill { height: 100%; border-radius: 2px; transition: width 0.4s ease; }
    .frust-bar-fill.low { background: var(--accent-green); }
    .frust-bar-fill.medium { background: var(--accent-amber); }
    .frust-bar-fill.high { background: var(--accent-red); }
    .frust-value { font-size: 12px; font-family: var(--font-mono); font-weight: 500; min-width: 32px; text-align: right; }

    /* ── Session Detail Sidebar ── */
    .session-layout { display: flex; gap: 24px; height: 100%; }
    .session-timeline { flex: 1; display: flex; flex-direction: column; gap: 16px; overflow-y: auto; padding-right: 12px; }
    .session-meta { width: 320px; background: var(--bg-surface); border: 1px solid var(--border); border-radius: var(--radius-md); padding: 24px; height: fit-content; }
    
    .meta-group { margin-bottom: 24px; }
    .meta-group-title { font-size: 12px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; margin-bottom: 12px; }
    .meta-row { display: flex; justify-content: space-between; font-size: 13px; margin-bottom: 8px; }
    .meta-row .label { color: var(--text-secondary); }
    .meta-row .val { color: var(--text-primary); font-family: var(--font-mono); }

    /* ── Timeline items ── */
    .turn-card { background: var(--bg-surface); border: 1px solid var(--border); border-radius: var(--radius-md); overflow: hidden; margin-bottom: 24px; }
    .turn-header { padding: 12px 20px; background: var(--bg-elevated); border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; font-size: 12px; color: var(--text-secondary); font-family: var(--font-mono); }
    .turn-body { padding: 20px; display: flex; flex-direction: column; gap: 16px; }
    
    .chat-msg {
      padding: 16px; border-radius: var(--radius-md);
      font-size: 14px; line-height: 1.6; white-space: pre-wrap;
    }
    .chat-user { background: var(--bg-elevated); color: var(--text-primary); border-left: 3px solid var(--accent-blue); }
    .chat-model { background: rgba(16,185,129,0.05); color: var(--text-primary); border-left: 3px solid var(--accent-green); }
    
    /* ── Tool calls ── */
    .tool-list { display: flex; flex-direction: column; gap: 8px; margin-top: 8px; }
    .tool-card { background: var(--bg-base); border: 1px solid var(--border); border-radius: var(--radius-sm); font-family: var(--font-mono); font-size: 12px; overflow: hidden; }
    .tool-card-head { padding: 8px 12px; background: var(--bg-elevated); border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; cursor: pointer; }
    .tool-card-head:hover { background: var(--bg-highlight); }
    .tool-card-body { padding: 12px; display: none; background: #000; color: #a1a1a1; white-space: pre-wrap; overflow-x: auto; }
    
    /* ── Signals ── */
    .signal-card { margin-top: 8px; padding: 12px; background: rgba(239,68,68,0.1); border: 1px solid rgba(239,68,68,0.2); border-radius: var(--radius-sm); border-left: 3px solid var(--accent-red); font-size: 13px; }
    .signal-title { font-weight: 600; color: var(--accent-red); margin-bottom: 4px; }
    
    /* ── Empty State ── */
    .empty { padding: 64px 20px; text-align: center; color: var(--text-secondary); }
    .empty-icon { font-size: 32px; margin-bottom: 16px; opacity: 0.5; }
    .empty-title { font-size: 16px; font-weight: 500; color: var(--text-primary); margin-bottom: 8px; }
    
  </style>
</head>
<body>

  <div class="header">
    <div class="logo">
      <div class="logo-icon">AL</div>
      <span>AgentLens</span>
      <span class="logo-sub">Observability</span>
    </div>
    <div class="header-actions">
      <div class="badge-live">Live Stream Active</div>
    </div>
  </div>

  <div class="nav-tabs">
    <button class="nav-tab active" onclick="nav('overview')">Overview</button>
    <button class="nav-tab" onclick="nav('sessions')">Sessions</button>
    <button class="nav-tab" onclick="nav('frustration')">Frustration Events</button>
    <button class="nav-tab" onclick="nav('hallucinations')">Hallucinations</button>
  </div>

  <div class="main">
    <div class="content">
      
      <!-- PAGE: OVERVIEW -->
      <div id="page-overview" class="page active">
        <div class="stat-grid" id="ov-stats">
          <!-- Populated by JS -->
        </div>
        <div class="section-header">
          <div class="section-title">Recent Sessions</div>
          <button class="nav-tab" style="padding: 4px 12px; font-size: 12px; border: 1px solid var(--border);" onclick="nav('sessions')">View All</button>
        </div>
        <div class="table-wrap" id="ov-sessions-table"></div>
      </div>

      <!-- PAGE: SESSIONS -->
      <div id="page-sessions" class="page">
        <div class="section-header">
          <div class="section-title">All Sessions</div>
        </div>
        <div class="table-wrap" id="sessions-table-full"></div>
      </div>

      <!-- PAGE: SESSION DETAIL -->
      <div id="page-session-detail" class="page">
        <div class="section-header">
          <div class="section-title" id="sd-title">Session Detail</div>
          <button class="nav-tab" style="padding: 4px 12px; font-size: 12px; border: 1px solid var(--border);" onclick="nav('sessions')">Back to Sessions</button>
        </div>
        <div class="session-layout">
          <div class="session-timeline" id="sd-timeline"></div>
          <div class="session-meta" id="sd-meta"></div>
        </div>
      </div>

      <!-- PAGE: FRUSTRATION -->
      <div id="page-frustration" class="page">
        <div class="section-header">
          <div class="section-title">Frustration Events</div>
        </div>
        <div class="table-wrap" id="frustration-table"></div>
      </div>

      <!-- PAGE: HALLUCINATIONS -->
      <div id="page-hallucinations" class="page">
        <div class="section-header">
          <div class="section-title">Hallucination Signals</div>
        </div>
        <div class="table-wrap" id="hallucinations-table"></div>
      </div>

    </div>
  </div>

<script>
let allSessions = [];
let currentSessionID = null;
let currentPage = 'overview';

function nav(page) {
  document.querySelectorAll('.page').forEach(el => el.classList.remove('active'));
  document.querySelectorAll('.nav-tab').forEach(el => el.classList.remove('active'));
  document.getElementById('page-' + page).classList.add('active');
  const tabs = document.querySelectorAll('.nav-tabs .nav-tab');
  
  if(page === 'overview') { tabs[0].classList.add('active'); loadOverview(); }
  else if(page === 'sessions') { tabs[1].classList.add('active'); renderSessionTable('sessions-table-full'); }
  else if(page === 'frustration') { tabs[2].classList.add('active'); renderFrustrationTable(); }
  else if(page === 'hallucinations') { tabs[3].classList.add('active'); renderHallucinations(); }
  currentPage = page;
}

// ─── API Fetches ───
async function fetchAPI(path) {
  try {
    const r = await fetch('/api/v1' + path);
    if(!r.ok) throw new Error('API err ' + r.status);
    return await r.json();
  } catch(e) {
    console.error(e);
    return null;
  }
}

async function loadOverview() {
  const health = await fetchAPI('/health/global?window=720h');
  if (health) {
    document.getElementById('ov-stats').innerHTML = ` + "`" + `
      <div class="stat-card"><div class="stat-label">Total Sessions</div><div class="stat-value">${health.total_sessions}</div></div>
      <div class="stat-card"><div class="stat-label">Success Rate</div><div class="stat-value green">${(health.success_rate*100).toFixed(1)}%</div></div>
      <div class="stat-card"><div class="stat-label">Avg Frustration</div><div class="stat-value ${health.avg_frustration_score>0.5?'red':'amber'}">${health.avg_frustration_score.toFixed(2)}</div></div>
      <div class="stat-card"><div class="stat-label">Hallucinations</div><div class="stat-value ${health.hallucination_rate>0.1?'red':''}">${(health.hallucination_rate*100).toFixed(1)}%</div></div>
    ` + "`" + `;
  }
  
  const sess = await fetchAPI('/sessions?limit=10');
  if (sess) {
    allSessions = sess;
    renderSessionTable('ov-sessions-table');
  }
}

function renderSessionTable(containerId) {
  const el = document.getElementById(containerId);
  if (!allSessions.length) {
    el.innerHTML = emptyState("No sessions yet", "Waiting for agent activity.");
    return;
  }
  
  const rows = allSessions.map(s => {
    // Heuristic status display (AI can override this later)
    let statusBadge = ` + "`" + `<span class="badge ${outcomeClass(s.outcome)}">${s.outcome}</span>` + "`" + `;
    if (s.outcome === 'in_progress') {
        // Just rename in_progress to Active for better UX right now
        statusBadge = ` + "`" + `<span class="badge info">Active</span>` + "`" + `;
    } else if (s.outcome === 'success') {
        statusBadge = ` + "`" + `<span class="badge success">Success</span>` + "`" + `;
    }
    
    const frust = s.frustration_score || 0;
    const fc = frust > 0.6 ? 'high' : frust > 0.3 ? 'medium' : 'low';
    const tokens = (s.total_tokens_in || 0) + (s.total_tokens_out || 0);
    
    return ` + "`" + `
      <tr onclick="openSession('${s.id}')">
        <td><code>${s.id.slice(0,8)}</code></td>
        <td><span class="badge neutral">${s.agent_id}</span></td>
        <td><span style="font-size:12px;color:var(--text-muted)">${s.model}</span></td>
        <td>
          <div class="frust-bar-wrap">
            <div class="frust-bar"><div class="frust-bar-fill ${fc}" style="width:${Math.round(frust*100)}%"></div></div>
            <span class="frust-value" style="color:${fc==='high'?'var(--accent-red)':'var(--accent-amber)'}">${frust.toFixed(2)}</span>
          </div>
        </td>
        <td>${tokens.toLocaleString()}</td>
        <td>${statusBadge}</td>
        <td style="color:var(--text-muted);font-size:12px">${timeAgo(s.started_at)}</td>
      </tr>
    ` + "`" + `;
  }).join('');

  el.innerHTML = ` + "`" + `
    <table>
      <thead><tr><th>ID</th><th>Agent</th><th>Model</th><th>Frustration</th><th>Tokens</th><th>Status</th><th>Time</th></tr></thead>
      <tbody>${rows}</tbody>
    </table>
  ` + "`" + `;
}

async function openSession(id) {
  currentSessionID = id;
  nav('session-detail');
  document.getElementById('sd-timeline').innerHTML = '';
  
  const s = allSessions.find(x => x.id === id);
  if(s) {
    document.getElementById('sd-title').innerText = ` + "`" + `Session ${id.slice(0,8)}` + "`" + `;
    document.getElementById('sd-meta').innerHTML = ` + "`" + `
      <div class="meta-group">
        <div class="meta-group-title">Details</div>
        <div class="meta-row"><span class="label">Agent</span><span class="val">${s.agent_id}</span></div>
        <div class="meta-row"><span class="label">Model</span><span class="val">${s.model}</span></div>
        <div class="meta-row"><span class="label">Tokens</span><span class="val">${s.total_tokens_in + s.total_tokens_out}</span></div>
        <div class="meta-row"><span class="label">Turns</span><span class="val">${s.turn_count}</span></div>
        <div class="meta-row"><span class="label">Outcome</span><span class="val"><span class="badge ${outcomeClass(s.outcome)}">${s.outcome}</span></span></div>
      </div>
      ${s.metadata && s.metadata.evaluation_summary ? ` + "`" + `
      <div class="meta-group" style="background: rgba(0, 112, 243, 0.05); border: 1px solid rgba(0, 112, 243, 0.2); padding: 16px; border-radius: var(--radius-sm); border-left: 3px solid var(--accent-blue);">
        <div class="meta-group-title" style="color: var(--accent-blue); margin-bottom: 8px; font-size: 11px;">AI EVALUATION SUMMARY</div>
        <div style="font-size: 13px; line-height: 1.5; color: var(--text-primary);">${escapeHTML(s.metadata.evaluation_summary)}</div>
      </div>` + "`" + ` : ''}
    ` + "`" + `;
  }
  
  const entries = await fetchAPI('/sessions/' + id + '/timeline');
  renderTimeline(entries);
}

// ─── DOM Update Utilities ───
function updateOrCreateTurn(t, toolsHtml, sigsHtml) {
  const domId = 'turn-' + t.id;
  let turnEl = document.getElementById(domId);
  const content = ` + "`" + `
    <div class="turn-header">
      <span>Turn ${t.index}</span>
      <span>${t.latency_ms}ms • ${t.tokens_in}in ${t.tokens_out}out</span>
    </div>
    <div class="turn-body">
      ${t.user_message ? ` + "`" + `<div class="chat-msg chat-user">${escapeHTML(t.user_message)}</div>` + "`" + ` : ''}
      ${t.model_response ? ` + "`" + `<div class="chat-msg chat-model">${escapeHTML(t.model_response)}</div>` + "`" + ` : ''}
      ${toolsHtml}
      ${sigsHtml}
    </div>
  ` + "`" + `;

  if (turnEl) {
    turnEl.innerHTML = content;
  } else {
    turnEl = document.createElement('div');
    turnEl.className = 'turn-card';
    turnEl.id = domId;
    turnEl.innerHTML = content;
    document.getElementById('sd-timeline').appendChild(turnEl);
  }
}

function renderTimeline(entries) {
  const el = document.getElementById('sd-timeline');
  if(!entries || !entries.length) {
    el.innerHTML = emptyState("No turns", "Timeline is empty.");
    return;
  }
  
  // Clear empty state if it's there
  if (el.querySelector('.empty')) {
    el.innerHTML = '';
  }
  
  for(const e of entries) {
    const t = e.turn;
    
    // Tools
    let toolsHtml = '';
    if (e.tool_calls && e.tool_calls.length) {
      const toolRows = e.tool_calls.map(tc => ` + "`" + `
        <div class="tool-card">
          <div class="tool-card-head" onclick="this.nextElementSibling.style.display = this.nextElementSibling.style.display==='block'?'none':'block'">
            <span>${categoryIcon(tc.category)} ${tc.tool_name}</span>
            <span class="badge ${tc.status==='success'?'success':'error'}">${tc.status} ${tc.duration_ms}ms</span>
          </div>
          <div class="tool-card-body">PARAMS:\n${tc.params||'{}'}\n\nRESULT:\n${tc.result||'{}'}</div>
        </div>
      ` + "`" + `).join('');
      toolsHtml = ` + "`" + `<div class="tool-list">${toolRows}</div>` + "`" + `;
    }
    
    // Signals
    let sigsHtml = '';
    if (e.signals && e.signals.length) {
      sigsHtml = e.signals.map(sig => {
        let h = '<div class="signal-card">';
        h += '<div class="signal-title">🚨 Hallucination Detected: ' + (sig.type ? sig.type.replace('_',' ') : '') + '</div>';
        if (sig.model_claim) h += '<div>Claim: ' + sig.model_claim + '</div>';
        if (sig.actual_value) h += '<div>Actual: ' + sig.actual_value + '</div>';
        if (sig.evidence) h += '<div>Evidence: ' + escapeHTML(sig.evidence) + '</div>';
        h += '</div>';
        return h;
      }).join('');
    }
    
    updateOrCreateTurn(t, toolsHtml, sigsHtml);
  }
}

function renderFrustrationTable() {
  const el = document.getElementById('frustration-table');
  const frustrated = (allSessions || []).filter(s => s.frustration_score > 0.3)
    .sort((a,b) => b.frustration_score - a.frustration_score);

  if (!frustrated.length) {
    el.innerHTML = emptyState('No frustration events', 'No sessions with elevated frustration scores.');
    return;
  }

  const rows = frustrated.map(s => {
    let statusBadge = ` + "`" + `<span class="badge ${outcomeClass(s.outcome)}">${s.outcome}</span>` + "`" + `;
    if (s.outcome === 'in_progress') statusBadge = ` + "`" + `<span class="badge info">Active</span>` + "`" + `;
    else if (s.outcome === 'success') statusBadge = ` + "`" + `<span class="badge success">Success</span>` + "`" + `;
    
    const frust = s.frustration_score;
    const fc = frust > 0.6 ? 'high' : 'medium';
    return ` + "`" + `
      <tr onclick="openSession('${s.id}')">
        <td><code>${s.id.slice(0,8)}</code></td>
        <td><span class="badge neutral">${s.agent_id}</span></td>
        <td>
          <div class="frust-bar-wrap">
            <div class="frust-bar"><div class="frust-bar-fill ${fc}" style="width:${Math.round(frust*100)}%"></div></div>
            <span class="frust-value" style="color:${fc==='high'?'var(--accent-red)':'var(--accent-amber)'}">${frust.toFixed(2)}</span>
          </div>
        </td>
        <td>${statusBadge}</td>
        <td style="color:var(--text-muted);font-size:12px">${timeAgo(s.started_at)}</td>
      </tr>
    ` + "`" + `;
  }).join('');

  el.innerHTML = ` + "`" + `
    <table>
      <thead><tr><th>ID</th><th>Agent</th><th>Frustration</th><th>Status</th><th>Time</th></tr></thead>
      <tbody>${rows}</tbody>
    </table>
  ` + "`" + `;
}

function renderHallucinations() {
  const el = document.getElementById('hallucinations-table');
  const rows = (allSessions || []).map(s => {
    let statusBadge = ` + "`" + `<span class="badge ${outcomeClass(s.outcome)}">${s.outcome}</span>` + "`" + `;
    if (s.outcome === 'in_progress') statusBadge = ` + "`" + `<span class="badge info">Active</span>` + "`" + `;
    else if (s.outcome === 'success') statusBadge = ` + "`" + `<span class="badge success">Success</span>` + "`" + `;
    
    return ` + "`" + `
      <tr onclick="openSession('${s.id}')">
        <td><code>${s.id.slice(0,8)}</code></td>
        <td><span class="badge neutral">${s.agent_id}</span></td>
        <td>Turns: ${s.turn_count}</td>
        <td>${statusBadge}</td>
        <td style="color:var(--text-muted);font-size:12px">${timeAgo(s.started_at)}</td>
      </tr>
    ` + "`" + `;
  }).join('');

  el.innerHTML = ` + "`" + `
    <table>
      <thead><tr><th>ID</th><th>Agent</th><th>Risk / Turns</th><th>Status</th><th>Time</th></tr></thead>
      <tbody>${rows}</tbody>
    </table>
  ` + "`" + `;
}

// ─── Utilities ───
function emptyState(title, sub) {
  return ` + "`" + `<div class="empty"><div class="empty-icon">🔭</div><div class="empty-title">${title}</div><div class="empty-sub">${sub}</div></div>` + "`" + `;
}

function outcomeClass(o) {
  return { success:'success', failed:'error', abandoned:'error', in_progress:'info', escalated:'warning' }[o] || 'neutral';
}
function categoryIcon(c) {
  return { file_ops:'📄', http:'🌐', database:'🗄️', compute:'⚙️', custom:'🔧' }[c] || '🔧';
}
function escapeHTML(str) {
  return str.replace(/[&<>'"]/g, tag => ({'&': '&amp;','<': '&lt;','>': '&gt;',"'": '&#39;','"': '&quot;'}[tag] || tag));
}
function timeAgo(ts) {
  const diff = (Date.now() - new Date(ts)) / 1000;
  if (diff < 60) return Math.round(diff) + 's ago';
  if (diff < 3600) return Math.round(diff/60) + 'm ago';
  return Math.round(diff/3600) + 'h ago';
}

// ─── SSE Live Updates ───
let overviewDebounce = null;
let detailDebounce = null;

function connectSSE() {
  const es = new EventSource('/api/v1/stream');
  
  es.addEventListener('session', async (e) => {
    const sessionID = e.data;
    
    if (currentPage === 'session-detail' && currentSessionID === sessionID) {
      clearTimeout(detailDebounce);
      detailDebounce = setTimeout(async () => {
        const entries = await fetchAPI('/sessions/' + sessionID + '/timeline');
        if (entries) renderTimeline(entries);
      }, 500); // Debounce detail fetches
    } else {
      clearTimeout(overviewDebounce);
      overviewDebounce = setTimeout(() => {
        loadOverview();
      }, 1000); // Debounce overview fetches
    }
  });

  es.onerror = () => { 
    console.error("SSE connection error. Browser will automatically retry."); 
  };
}

document.addEventListener('DOMContentLoaded', () => {
  loadOverview();
  connectSSE();
});
</script>
</body>
</html>
`
