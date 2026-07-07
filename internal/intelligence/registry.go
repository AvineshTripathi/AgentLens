package intelligence

import (
	"context"
	"time"

	"github.com/AvineshTripathi/AgentLens/internal/store"
	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// AnalysisContext provides all necessary data for an Analyzer to evaluate a turn.
type AnalysisContext struct {
	Turn               *types.Turn
	Session            *types.Session
	Store              *store.Store
	PreviousTurnAt     *time.Time
	RecentUserMessages []string
}

// Analyzer is the core interface for an injectable intelligence module.
type Analyzer interface {
	Name() string
	Analyze(ctx context.Context, actx *AnalysisContext) error
}

// Registry manages and executes a collection of Analyzers in sequence.
type Registry struct {
	analyzers []Analyzer
}

// NewRegistry creates an empty intelligence registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds an Analyzer to the pipeline.
func (r *Registry) Register(a Analyzer) {
	r.analyzers = append(r.analyzers, a)
}

// AnalyzeAll executes all registered analyzers on the given context.
func (r *Registry) AnalyzeAll(ctx context.Context, actx *AnalysisContext) {
	for _, a := range r.analyzers {
		// Log errors but continue the pipeline
		_ = a.Analyze(ctx, actx)
	}
}
