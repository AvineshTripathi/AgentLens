-- AgentLens — Evaluator Migration
-- Adds evaluated_at to track which sessions have been judged.

BEGIN;

ALTER TABLE sessions
ADD COLUMN IF NOT EXISTS evaluated_at TIMESTAMPTZ;

-- Index to quickly find pending evaluations
CREATE INDEX IF NOT EXISTS idx_sessions_pending_eval 
ON sessions (ended_at, evaluated_at)
WHERE evaluated_at IS NULL AND outcome != 'in_progress';

COMMIT;
