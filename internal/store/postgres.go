// Package store implements PostgreSQL persistence for all AgentLens signals.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// Store handles all reads and writes to PostgreSQL.
type Store struct {
	db *sql.DB
}

// New creates a new Store and verifies the connection.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database connection pool.
func (s *Store) Close() error { return s.db.Close() }

// ─── Session ──────────────────────────────────────────────────────────────

// UpsertSession inserts or updates a session record.
func (s *Store) UpsertSession(ctx context.Context, sess *types.Session) error {
	meta, _ := json.Marshal(sess.Metadata)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, user_id, agent_id, provider, model,
			started_at, ended_at, outcome,
			turn_count, total_tokens_in, total_tokens_out,
			frustration_score, metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (id) DO UPDATE SET
			ended_at          = EXCLUDED.ended_at,
			outcome           = EXCLUDED.outcome,
			turn_count        = EXCLUDED.turn_count,
			total_tokens_in   = EXCLUDED.total_tokens_in,
			total_tokens_out  = EXCLUDED.total_tokens_out,
			frustration_score = EXCLUDED.frustration_score,
			metadata          = EXCLUDED.metadata`,
		sess.ID, nullString(sess.UserID), sess.AgentID,
		string(sess.Provider), sess.Model,
		sess.StartedAt, sess.EndedAt, string(sess.Outcome),
		sess.TurnCount, sess.TotalTokensIn, sess.TotalTokensOut,
		sess.FrustrationScore, meta,
	)
	return err
}

// GetSession retrieves a session by ID.
func (s *Store) GetSession(ctx context.Context, id string) (*types.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, agent_id, provider, model,
		        started_at, ended_at, outcome,
		        turn_count, total_tokens_in, total_tokens_out,
		        frustration_score, metadata
		 FROM sessions WHERE id = $1`, id)

	var sess types.Session
	var userID sql.NullString
	var endedAt sql.NullTime
	var meta []byte

	err := row.Scan(
		&sess.ID, &userID, &sess.AgentID, &sess.Provider, &sess.Model,
		&sess.StartedAt, &endedAt, &sess.Outcome,
		&sess.TurnCount, &sess.TotalTokensIn, &sess.TotalTokensOut,
		&sess.FrustrationScore, &meta,
	)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		sess.UserID = userID.String
	}
	if endedAt.Valid {
		sess.EndedAt = &endedAt.Time
	}
	if meta != nil {
		_ = json.Unmarshal(meta, &sess.Metadata)
	}
	return &sess, nil
}

// ListSessions returns recent sessions, newest first.
func (s *Store) ListSessions(ctx context.Context, limit int) ([]*types.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, agent_id, provider, model,
		        started_at, ended_at, outcome,
		        turn_count, total_tokens_in, total_tokens_out,
		        frustration_score, metadata
		 FROM sessions
		 ORDER BY (SELECT COALESCE(MAX(created_at), sessions.started_at) FROM turns WHERE turns.session_id = sessions.id) DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*types.Session
	for rows.Next() {
		var sess types.Session
		var userID sql.NullString
		var endedAt sql.NullTime
		var meta []byte
		if err := rows.Scan(
			&sess.ID, &userID, &sess.AgentID, &sess.Provider, &sess.Model,
			&sess.StartedAt, &endedAt, &sess.Outcome,
			&sess.TurnCount, &sess.TotalTokensIn, &sess.TotalTokensOut,
			&sess.FrustrationScore, &meta,
		); err != nil {
			return nil, err
		}
		if userID.Valid {
			sess.UserID = userID.String
		}
		if endedAt.Valid {
			sess.EndedAt = &endedAt.Time
		}
		if meta != nil {
			_ = json.Unmarshal(meta, &sess.Metadata)
		}
		sessions = append(sessions, &sess)
	}
	return sessions, rows.Err()
}

// ─── Turn ─────────────────────────────────────────────────────────────────

// InsertTurn writes a new conversation turn.
func (s *Store) InsertTurn(ctx context.Context, t *types.Turn) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO turns (
			id, session_id, turn_index,
			user_message, model_response, thinking_trace,
			tokens_in, tokens_out, latency_ms,
			frustration_delta, hallucination_risk,
			created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		t.ID, t.SessionID, t.Index,
		t.UserMessage, t.ModelResponse, t.ThinkingTrace,
		t.TokensIn, t.TokensOut, t.LatencyMs,
		t.FrustrationDelta, t.HallucinationRisk,
		t.CreatedAt,
	)
	return err
}

// UpdateTurn updates an existing conversation turn (used for aggregating tool loops).
func (s *Store) UpdateTurn(ctx context.Context, t *types.Turn) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE turns SET
			model_response = $1,
			tokens_in = $2,
			tokens_out = $3,
			latency_ms = $4,
			frustration_delta = $5,
			hallucination_risk = $6
		WHERE id = $7`,
		t.ModelResponse,
		t.TokensIn, t.TokensOut, t.LatencyMs,
		t.FrustrationDelta, t.HallucinationRisk,
		t.ID,
	)
	return err
}

// ListTurns returns all turns for a session, in order.
func (s *Store) ListTurns(ctx context.Context, sessionID string) ([]*types.Turn, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, turn_index,
		        user_message, model_response, thinking_trace,
		        tokens_in, tokens_out, latency_ms,
		        frustration_delta, hallucination_risk, created_at
		 FROM turns WHERE session_id = $1 ORDER BY turn_index ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turns []*types.Turn
	for rows.Next() {
		var t types.Turn
		if err := rows.Scan(
			&t.ID, &t.SessionID, &t.Index,
			&t.UserMessage, &t.ModelResponse, &t.ThinkingTrace,
			&t.TokensIn, &t.TokensOut, &t.LatencyMs,
			&t.FrustrationDelta, &t.HallucinationRisk, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		turns = append(turns, &t)
	}
	return turns, rows.Err()
}

// ListTimelineTurns returns turns with user and model messages heavily truncated
// to drastically reduce JSON payload size and browser rendering overhead for the timeline view.
func (s *Store) ListTimelineTurns(ctx context.Context, sessionID string) ([]*types.Turn, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, turn_index,
		        LEFT(user_message, 250) AS user_message, 
		        LEFT(model_response, 250) AS model_response, 
		        thinking_trace,
		        tokens_in, tokens_out, latency_ms,
		        frustration_delta, hallucination_risk, created_at
		 FROM turns WHERE session_id = $1 ORDER BY turn_index ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turns []*types.Turn
	for rows.Next() {
		var t types.Turn
		if err := rows.Scan(
			&t.ID, &t.SessionID, &t.Index,
			&t.UserMessage, &t.ModelResponse, &t.ThinkingTrace,
			&t.TokensIn, &t.TokensOut, &t.LatencyMs,
			&t.FrustrationDelta, &t.HallucinationRisk, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		turns = append(turns, &t)
	}
	return turns, rows.Err()
}

// ─── Tool Call ────────────────────────────────────────────────────────────

// InsertToolCall writes a tool call record (pre-execution, status=running).
func (s *Store) InsertToolCall(ctx context.Context, tc *types.ToolCall) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_calls (
			id, turn_id, session_id, trace_id, span_id,
			tool_name, category, params, result,
			status, error_message, duration_ms,
			started_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		tc.ID, tc.TurnID, tc.SessionID, tc.TraceID, tc.SpanID,
		tc.ToolName, string(tc.Category), tc.Params, tc.Result,
		string(tc.Status), nullString(tc.ErrorMessage), tc.DurationMs,
		tc.StartedAt, tc.CompletedAt,
	)
	return err
}

// UpdateToolCall updates a tool call with result + completion info.
func (s *Store) UpdateToolCall(ctx context.Context, tc *types.ToolCall) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tool_calls SET
			result        = $2,
			status        = $3,
			error_message = $4,
			duration_ms   = $5,
			completed_at  = $6
		WHERE id = $1`,
		tc.ID, tc.Result, string(tc.Status),
		nullString(tc.ErrorMessage), tc.DurationMs, tc.CompletedAt,
	)
	return err
}

// ListToolCalls returns all tool calls for a session.
func (s *Store) ListToolCalls(ctx context.Context, sessionID string) ([]*types.ToolCall, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, turn_id, session_id, trace_id, span_id,
		        tool_name, category, params, result,
		        status, error_message, duration_ms, started_at, completed_at
		 FROM tool_calls WHERE session_id = $1 ORDER BY started_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []*types.ToolCall
	for rows.Next() {
		var tc types.ToolCall
		var errMsg sql.NullString
		var completedAt sql.NullTime
		if err := rows.Scan(
			&tc.ID, &tc.TurnID, &tc.SessionID, &tc.TraceID, &tc.SpanID,
			&tc.ToolName, &tc.Category, &tc.Params, &tc.Result,
			&tc.Status, &errMsg, &tc.DurationMs, &tc.StartedAt, &completedAt,
		); err != nil {
			return nil, err
		}
		if errMsg.Valid {
			tc.ErrorMessage = errMsg.String
		}
		if completedAt.Valid {
			tc.CompletedAt = &completedAt.Time
		}
		calls = append(calls, &tc)
	}
	return calls, rows.Err()
}

// ─── Hallucination ────────────────────────────────────────────────────────

// InsertHallucinationSignal records a detected hallucination signal.
func (s *Store) InsertHallucinationSignal(ctx context.Context, h *types.HallucinationSignal) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO hallucination_signals (
			id, session_id, turn_id, signal_type,
			risk_score, model_claim, actual_value, evidence, detected_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		h.ID, h.SessionID, h.TurnID, string(h.Type),
		h.RiskScore, h.ModelClaim, h.ActualValue, h.Evidence, h.DetectedAt,
	)
	return err
}

// ListHallucinationSignals returns signals for a session.
func (s *Store) ListHallucinationSignals(ctx context.Context, sessionID string) ([]*types.HallucinationSignal, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, turn_id, signal_type,
		        risk_score, model_claim, actual_value, evidence, detected_at
		 FROM hallucination_signals WHERE session_id = $1 ORDER BY detected_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var signals []*types.HallucinationSignal
	for rows.Next() {
		var h types.HallucinationSignal
		if err := rows.Scan(
			&h.ID, &h.SessionID, &h.TurnID, &h.Type,
			&h.RiskScore, &h.ModelClaim, &h.ActualValue, &h.Evidence, &h.DetectedAt,
		); err != nil {
			return nil, err
		}
		signals = append(signals, &h)
	}
	return signals, rows.Err()
}

// ─── Frustration ─────────────────────────────────────────────────────────

// InsertFrustrationEvent records a frustration spike.
func (s *Store) InsertFrustrationEvent(ctx context.Context, fe *types.FrustrationEvent) error {
	triggers, _ := json.Marshal(fe.Triggers)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO frustration_events (
			id, session_id, turn_id, score, triggers, user_message_snip, detected_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		fe.ID, fe.SessionID, fe.TurnID, fe.Score,
		triggers, fe.UserMessageSnip, fe.DetectedAt,
	)
	return err
}

// ─── Infra Events ─────────────────────────────────────────────────────────

// InsertInfraEvent records an infrastructure-level event.
func (s *Store) InsertInfraEvent(ctx context.Context, ie *types.InfraEvent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO infra_events (id, service, event_type, duration_ms, error_msg, occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		ie.ID, ie.Service, string(ie.EventType),
		ie.DurationMs, nullString(ie.ErrorMsg), ie.OccurredAt,
	)
	return err
}

// ─── Agent Health ─────────────────────────────────────────────────────────

// GetAgentHealth computes an aggregated health snapshot from DB.
func (s *Store) GetAgentHealth(ctx context.Context, agentID string, since time.Duration) (*types.AgentHealth, error) {
	windowStart := time.Now().Add(-since)
	windowEnd := time.Now()

	health := &types.AgentHealth{
		AgentID:     agentID,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE outcome = 'success') AS successful,
			COUNT(*) FILTER (WHERE outcome = 'abandoned') AS abandoned,
			COALESCE(AVG(frustration_score), 0) AS avg_frustration,
			COALESCE(AVG(turn_count), 0) AS avg_turns
		FROM sessions
		WHERE agent_id = $1 AND started_at >= $2`,
		agentID, windowStart,
	).Scan(
		&health.TotalSessions,
		&health.SuccessfulSessions,
		&health.AbandonedSessions,
		&health.AvgFrustrationScore,
		&health.AvgSessionTurns,
	)
	if err != nil {
		return nil, err
	}

	if health.TotalSessions > 0 {
		health.SuccessRate = float64(health.SuccessfulSessions) / float64(health.TotalSessions)
	}
	return health, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
