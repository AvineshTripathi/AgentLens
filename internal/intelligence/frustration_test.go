package intelligence

import (
	"testing"
)

func TestContainsAny(t *testing.T) {
	if !containsAny("this is terrible", []string{"terrible", "bad"}) {
		t.Errorf("Expected true for 'terrible'")
	}
	if containsAny("this is great", []string{"terrible", "bad"}) {
		t.Errorf("Expected false for 'great'")
	}
}

func TestSimilarityRatio(t *testing.T) {
	r := similarityRatio("why did you do that", "why did you do that")
	if r != 1.0 {
		t.Errorf("Expected 1.0 for exact match, got %v", r)
	}

	r2 := similarityRatio("why did you do that", "can you write a script")
	if r2 > 0.5 {
		t.Errorf("Expected low similarity for completely different strings, got %v", r2)
	}
}
