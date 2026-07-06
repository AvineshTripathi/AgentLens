-- AgentLens — PostgreSQL Schema
-- Run with: psql $DSN -f migrations/001_init.sql

BEGIN;

-- ─── Sessions ─────────────────────────────────────────────────────────────
-- A session is the top-level unit — one full user↔agent interaction.
CREATE TABLE IF NOT EXISTS sessions (
    id                UUID        PRIMARY KEY,
    user_id           TEXT,
    agent_id          TEXT        NOT NULL,
    provider          TEXT        NOT NULL,   -- anthropic/openai/gemini/custom
    model             TEXT        NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL,
    ended_at          TIMESTAMPTZ,
    outcome           TEXT        NOT NULL DEFAULT 'in_progress',
                                             -- success/abandoned/escalated/failed/in_progress
    turn_count        INT         NOT NULL DEFAULT 0,
    total_tokens_in   INT         NOT NULL DEFAULT 0,
    total_tokens_out  INT         NOT NULL DEFAULT 0,
    frustration_score FLOAT       NOT NULL DEFAULT 0.0,
    metadata          JSONB
);

CREATE INDEX IF NOT EXISTS idx_sessions_agent_time   ON sessions (agent_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_outcome_time ON sessions (outcome, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_frustration  ON sessions (frustration_score DESC);

-- ─── Turns ────────────────────────────────────────────────────────────────
-- One request/response cycle between user and model within a session.
CREATE TABLE IF NOT EXISTS turns (
    id                 UUID        PRIMARY KEY,
    session_id         UUID        NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    turn_index         INT         NOT NULL,
    user_message       TEXT,
    model_response     TEXT,
    thinking_trace     TEXT,                  -- chain-of-thought if captured
    tokens_in          INT         NOT NULL DEFAULT 0,
    tokens_out         INT         NOT NULL DEFAULT 0,
    latency_ms         INT         NOT NULL DEFAULT 0,
    frustration_delta  FLOAT       NOT NULL DEFAULT 0.0,
    hallucination_risk FLOAT       NOT NULL DEFAULT 0.0,
    created_at         TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_turns_session ON turns (session_id, turn_index ASC);
CREATE INDEX IF NOT EXISTS idx_turns_hallucination ON turns (hallucination_risk DESC) WHERE hallucination_risk > 0.5;
CREATE INDEX IF NOT EXISTS idx_turns_frustration ON turns (frustration_delta DESC) WHERE frustration_delta > 0.3;

-- ─── Tool Calls ───────────────────────────────────────────────────────────
-- A single tool invocation within a turn.
CREATE TABLE IF NOT EXISTS tool_calls (
    id            UUID        PRIMARY KEY,
    turn_id       UUID        NOT NULL REFERENCES turns(id) ON DELETE CASCADE,
    session_id    UUID        NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    trace_id      TEXT,                      -- OTel trace ID
    span_id       TEXT,                      -- OTel span ID
    tool_name     TEXT        NOT NULL,
    category      TEXT        NOT NULL,      -- file_ops/http/database/compute/custom
    params        JSONB,                     -- sanitized input params
    result        JSONB,                     -- sanitized output
    status        TEXT        NOT NULL,      -- success/error/denied/timeout/running
    error_message TEXT,
    duration_ms   INT         NOT NULL DEFAULT 0,
    started_at    TIMESTAMPTZ NOT NULL,
    completed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_session    ON tool_calls (session_id, started_at ASC);
CREATE INDEX IF NOT EXISTS idx_tool_calls_turn       ON tool_calls (turn_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_name_time  ON tool_calls (tool_name, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_tool_calls_status     ON tool_calls (status) WHERE status != 'success';
CREATE INDEX IF NOT EXISTS idx_tool_calls_params     ON tool_calls USING gin (params);

-- ─── Hallucination Signals ────────────────────────────────────────────────
-- Records detected or suspected hallucinations.
CREATE TABLE IF NOT EXISTS hallucination_signals (
    id           UUID        PRIMARY KEY,
    session_id   UUID        NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    turn_id      UUID        NOT NULL REFERENCES turns(id) ON DELETE CASCADE,
    signal_type  TEXT        NOT NULL,   -- tool_contradiction/dead_reference/fabricated_action/etc
    risk_score   FLOAT       NOT NULL,   -- 0.0 → 1.0
    model_claim  TEXT,                   -- what the model said
    actual_value TEXT,                   -- what actually happened
    evidence     TEXT,
    detected_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_hallucination_session ON hallucination_signals (session_id, detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_hallucination_risk    ON hallucination_signals (risk_score DESC);
CREATE INDEX IF NOT EXISTS idx_hallucination_type    ON hallucination_signals (signal_type, detected_at DESC);

-- ─── Frustration Events ───────────────────────────────────────────────────
-- Notable spikes in user frustration.
CREATE TABLE IF NOT EXISTS frustration_events (
    id                UUID        PRIMARY KEY,
    session_id        UUID        NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    turn_id           UUID        NOT NULL REFERENCES turns(id) ON DELETE CASCADE,
    score             FLOAT       NOT NULL,
    triggers          JSONB,               -- array of trigger types that fired
    user_message_snip TEXT,               -- first 120 chars of triggering message
    detected_at       TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_frustration_session ON frustration_events (session_id, detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_frustration_score   ON frustration_events (score DESC);

-- ─── Infrastructure Events ────────────────────────────────────────────────
-- Infrastructure-level issues (DB timeouts, API errors, slowness).
CREATE TABLE IF NOT EXISTS infra_events (
    id          UUID        PRIMARY KEY,
    service     TEXT        NOT NULL,    -- "postgres", "redis", "openai-api", "s3"
    event_type  TEXT        NOT NULL,    -- timeout/rate_limit/error/slow
    duration_ms INT,
    error_msg   TEXT,
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_infra_service_time ON infra_events (service, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_infra_type_time    ON infra_events (event_type, occurred_at DESC);

-- ─── Infra Correlations ───────────────────────────────────────────────────
-- Links infra events to turns where they likely caused degraded output.
CREATE TABLE IF NOT EXISTS infra_correlations (
    id              UUID    PRIMARY KEY,
    infra_event_id  UUID    NOT NULL REFERENCES infra_events(id) ON DELETE CASCADE,
    turn_id         UUID    NOT NULL REFERENCES turns(id) ON DELETE CASCADE,
    session_id      UUID    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    confidence      FLOAT   NOT NULL,
    window_secs     INT     NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_infra_corr_session ON infra_correlations (session_id);

COMMIT;
