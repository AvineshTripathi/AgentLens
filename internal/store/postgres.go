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
			frustration_score, evaluated_at, metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (id) DO UPDATE SET
			ended_at          = EXCLUDED.ended_at,
			outcome           = EXCLUDED.outcome,
			turn_count        = EXCLUDED.turn_count,
			total_tokens_in   = EXCLUDED.total_tokens_in,
			total_tokens_out  = EXCLUDED.total_tokens_out,
			frustration_score = EXCLUDED.frustration_score,
			evaluated_at      = EXCLUDED.evaluated_at,
			metadata          = CASE WHEN EXCLUDED.metadata IS NULL OR EXCLUDED.metadata::text = 'null' THEN sessions.metadata ELSE EXCLUDED.metadata END`,
		sess.ID, nullString(sess.UserID), sess.AgentID,
		string(sess.Provider), sess.Model,
		sess.StartedAt, sess.EndedAt, string(sess.Outcome),
		sess.TurnCount, sess.TotalTokensIn, sess.TotalTokensOut,
		sess.FrustrationScore, sess.EvaluatedAt, meta,
	)
	return err
}

// GetSession retrieves a session by ID.
func (s *Store) GetSession(ctx context.Context, id string) (*types.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, agent_id, provider, model,
		        started_at, ended_at, outcome,
		        turn_count, total_tokens_in, total_tokens_out,
		        frustration_score, evaluated_at, metadata
		 FROM sessions WHERE id = $1`, id)

	var sess types.Session
	var userID sql.NullString
	var endedAt sql.NullTime
	var evalAt sql.NullTime
	var meta []byte

	err := row.Scan(
		&sess.ID, &userID, &sess.AgentID, &sess.Provider, &sess.Model,
		&sess.StartedAt, &endedAt, &sess.Outcome,
		&sess.TurnCount, &sess.TotalTokensIn, &sess.TotalTokensOut,
		&sess.FrustrationScore, &evalAt, &meta,
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
	if evalAt.Valid {
		sess.EvaluatedAt = &evalAt.Time
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
		        frustration_score, evaluated_at, metadata
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
		var evalAt sql.NullTime
		var meta []byte
		if err := rows.Scan(
			&sess.ID, &userID, &sess.AgentID, &sess.Provider, &sess.Model,
			&sess.StartedAt, &endedAt, &sess.Outcome,
			&sess.TurnCount, &sess.TotalTokensIn, &sess.TotalTokensOut,
			&sess.FrustrationScore, &evalAt, &meta,
		); err != nil {
			return nil, err
		}
		if userID.Valid {
			sess.UserID = userID.String
		}
		if endedAt.Valid {
			sess.EndedAt = &endedAt.Time
		}
		if evalAt.Valid {
			sess.EvaluatedAt = &evalAt.Time
		}
		if meta != nil {
			_ = json.Unmarshal(meta, &sess.Metadata)
		}
		sessions = append(sessions, &sess)
	}
	return sessions, rows.Err()
}

// GetPendingEvaluations returns sessions that are completed or idle and need evaluation.
func (s *Store) GetPendingEvaluations(ctx context.Context, limit int) ([]*types.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, agent_id, provider, model,
		        started_at, ended_at, outcome,
		        turn_count, total_tokens_in, total_tokens_out,
		        frustration_score, evaluated_at, metadata
		 FROM sessions
		 WHERE evaluated_at IS NULL AND (outcome != 'in_progress' OR 
		       (SELECT COALESCE(MAX(created_at), sessions.started_at) FROM turns WHERE turns.session_id = sessions.id) < NOW() - INTERVAL '1 minute')
		 ORDER BY started_at ASC
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
		var evalAt sql.NullTime
		var meta []byte
		if err := rows.Scan(
			&sess.ID, &userID, &sess.AgentID, &sess.Provider, &sess.Model,
			&sess.StartedAt, &endedAt, &sess.Outcome,
			&sess.TurnCount, &sess.TotalTokensIn, &sess.TotalTokensOut,
			&sess.FrustrationScore, &evalAt, &meta,
		); err != nil {
			return nil, err
		}
		if userID.Valid {
			sess.UserID = userID.String
		}
		if endedAt.Valid {
			sess.EndedAt = &endedAt.Time
		}
		if evalAt.Valid {
			sess.EvaluatedAt = &evalAt.Time
		}
		if meta != nil {
			_ = json.Unmarshal(meta, &sess.Metadata)
		}
		sessions = append(sessions, &sess)
	}
	return sessions, rows.Err()
}

// SaveEvaluation saves the LLM's judgement of a session.
func (s *Store) SaveEvaluation(ctx context.Context, sessionID string, outcome types.OutcomeStatus, frustration float64, hallucinationReason string, evaluationSummary string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET
			outcome = $2,
			frustration_score = $3,
			evaluated_at = NOW(),
			metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('evaluation_summary', $4::text)
		WHERE id = $1`,
		sessionID, string(outcome), frustration, evaluationSummary,
	)
	if err != nil {
		return err
	}

	if hallucinationReason != "" {
		// Attempt to get the last turn ID to attach the signal to
		var turnID string
		err := s.db.QueryRowContext(ctx, "SELECT id FROM turns WHERE session_id = $1 ORDER BY turn_index DESC LIMIT 1", sessionID).Scan(&turnID)
		if err == nil {
			_, _ = s.db.ExecContext(ctx, `
				INSERT INTO hallucination_signals (
					id, session_id, turn_id, signal_type,
					risk_score, model_claim, actual_value, evidence, detected_at
				) VALUES (gen_random_uuid(), $1, $2, 'evaluator_audit', 1.0, '', '', $3, NOW())
			`, sessionID, turnID, hallucinationReason)
		}
	}
	return nil
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

// ListTimelineEntries returns a fully populated timeline for a session in a single optimized query.
func (s *Store) ListTimelineEntries(ctx context.Context, sessionID string) ([]types.TimelineEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.session_id, t.turn_index,
		        LEFT(t.user_message, 250), LEFT(t.model_response, 250), t.thinking_trace,
		        t.tokens_in, t.tokens_out, t.latency_ms,
		        t.frustration_delta, t.hallucination_risk, t.created_at,
		        (SELECT COALESCE(json_agg(row_to_json(tc)), '[]') FROM tool_calls tc WHERE tc.turn_id = t.id) as tool_calls,
		        (SELECT COALESCE(json_agg(row_to_json(hs)), '[]') FROM hallucination_signals hs WHERE hs.turn_id = t.id) as signals
		 FROM turns t WHERE t.session_id = $1 ORDER BY t.turn_index ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []types.TimelineEntry
	for rows.Next() {
		var t types.Turn
		var toolCallsJSON, signalsJSON []byte

		if err := rows.Scan(
			&t.ID, &t.SessionID, &t.Index,
			&t.UserMessage, &t.ModelResponse, &t.ThinkingTrace,
			&t.TokensIn, &t.TokensOut, &t.LatencyMs,
			&t.FrustrationDelta, &t.HallucinationRisk, &t.CreatedAt,
			&toolCallsJSON, &signalsJSON,
		); err != nil {
			return nil, err
		}

		var toolCalls []*types.ToolCall
		var signals []*types.HallucinationSignal
		_ = json.Unmarshal(toolCallsJSON, &toolCalls)
		_ = json.Unmarshal(signalsJSON, &signals)

		entries = append(entries, types.TimelineEntry{
			Turn:      &t,
			ToolCalls: toolCalls,
			Signals:   signals,
		})
	}
	return entries, rows.Err()
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

	query := `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE outcome = 'success') AS successful,
			COUNT(*) FILTER (WHERE outcome = 'abandoned') AS abandoned,
			COALESCE(AVG(frustration_score), 0) AS avg_frustration,
			COALESCE(AVG(turn_count), 0) AS avg_turns
		FROM sessions
		WHERE started_at >= $1`

	var err error
	if agentID == "global" {
		err = s.db.QueryRowContext(ctx, query, windowStart).Scan(
			&health.TotalSessions,
			&health.SuccessfulSessions,
			&health.AbandonedSessions,
			&health.AvgFrustrationScore,
			&health.AvgSessionTurns,
		)
	} else {
		query += " AND agent_id = $2"
		err = s.db.QueryRowContext(ctx, query, windowStart, agentID).Scan(
			&health.TotalSessions,
			&health.SuccessfulSessions,
			&health.AbandonedSessions,
			&health.AvgFrustrationScore,
			&health.AvgSessionTurns,
		)
	}
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
