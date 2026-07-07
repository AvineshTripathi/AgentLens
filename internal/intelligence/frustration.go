package intelligence

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/AvineshTripathi/AgentLens/internal/types"
	"github.com/google/uuid"
)

// FrustrationAnalyzer implements the Analyzer interface.
// It scores user frustration per turn using deep behavioral and linguistic signals.
type FrustrationAnalyzer struct {
	RagePromptWindowSecs int
}

func NewFrustrationAnalyzer() *FrustrationAnalyzer {
	return &FrustrationAnalyzer{RagePromptWindowSecs: 8}
}

func (a *FrustrationAnalyzer) Name() string {
	return "FrustrationAnalyzer"
}

func (a *FrustrationAnalyzer) Analyze(ctx context.Context, actx *AnalysisContext) error {
	msg := actx.Turn.UserMessage
	if msg == "" {
		return nil
	}

	var triggers []types.FrustrationTrigger
	newPoints := 0.0
	lower := strings.ToLower(msg)

	// 1. Linguistic Signals
	if len(msg) > 8 && isAllCaps(msg) {
		triggers = append(triggers, types.TriggerAllCaps)
		newPoints += 0.20
	}

	if strings.Count(msg, "!") >= 3 {
		triggers = append(triggers, types.TriggerExclamations)
		newPoints += 0.15
	}

	negativeKeywords := []string{
		"useless", "wrong", "not working", "broken", "stupid", "idiot",
		"terrible", "horrible", "awful", "ridiculous", "pathetic",
		"disappointed", "frustrated", "angry", "annoying", "trash",
		"garbage", "nonsense", "pointless",
	}
	if containsAny(lower, negativeKeywords) {
		triggers = append(triggers, types.TriggerNegativeSentiment)
		newPoints += 0.25
	}

	abandonmentKeywords := []string{
		"forget it", "never mind", "give up", "forget this", "screw it",
		"this is pointless", "i quit", "done with this", "not worth it",
	}
	if containsAny(lower, abandonmentKeywords) {
		triggers = append(triggers, types.TriggerAbandonmentSignal)
		newPoints += 0.40
	}

	correctionPhrases := []string{
		"no, i said", "that's wrong", "i didn't ask", "you misunderstood",
		"that's not what i", "again,", "i already told you", "read it again",
		"pay attention", "listen to me", "stop doing that",
	}
	if containsAny(lower, correctionPhrases) {
		triggers = append(triggers, types.TriggerExplicitCorrection)
		newPoints += 0.30 // Increased weight for deep explicit correction
	}

	// 2. Behavioral Signals
	if actx.PreviousTurnAt != nil && time.Since(*actx.PreviousTurnAt) < time.Duration(a.RagePromptWindowSecs)*time.Second {
		triggers = append(triggers, types.TriggerRagePrompting)
		newPoints += 0.15
	}

	for _, prev := range actx.RecentUserMessages {
		if similarityRatio(lower, strings.ToLower(prev)) > 0.85 {
			triggers = append(triggers, types.TriggerRepeatedQuestion)
			newPoints += 0.30
			break
		}
	}

	// 3. Roll up score
	prevScore := actx.Session.FrustrationScore
	newScore := clamp(prevScore*0.70+newPoints, 0.0, 1.0)
	delta := newScore - prevScore

	actx.Turn.FrustrationDelta = delta
	actx.Session.FrustrationScore = newScore

	if newScore >= 0.85 {
		actx.Session.Outcome = types.OutcomeAbandoned
	}

	// 4. Persist event if threshold crossed
	if newScore >= 0.60 {
		fe := &types.FrustrationEvent{
			ID:              uuid.NewString(),
			SessionID:       actx.Session.ID,
			TurnID:          actx.Turn.ID,
			Score:           newScore,
			Triggers:        triggers,
			UserMessageSnip: truncate(msg, 120),
			DetectedAt:      time.Now(),
		}

		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := actx.Store.InsertFrustrationEvent(bgCtx, fe); err != nil {
				slog.Error("failed to persist frustration event", "err", err)
			}
		}()
	}

	return nil
}

func isAllCaps(s string) bool {
	hasLetters := false
	for _, c := range s {
		if c >= 'a' && c <= 'z' {
			return false
		}
		if c >= 'A' && c <= 'Z' {
			hasLetters = true
		}
	}
	return hasLetters
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// similarityRatio is a basic Jaccard similarity for words
func similarityRatio(s1, s2 string) float64 {
	w1 := strings.Fields(s1)
	w2 := strings.Fields(s2)
	if len(w1) == 0 && len(w2) == 0 {
		return 1.0
	}

	set1 := make(map[string]bool)
	for _, w := range w1 {
		set1[w] = true
	}

	intersection := 0
	for _, w := range w2 {
		if set1[w] {
			intersection++
			delete(set1, w)
		}
	}

	union := len(w1) + len(w2) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
