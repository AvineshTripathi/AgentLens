package intelligence

import (
	"testing"

	"github.com/AvineshTripathi/AgentLens/internal/types"
)

// A mock store that does not require a real postgres DB for testing.
// However, since Store is a concrete type in internal/store, we may have to deal with nil store panics if it is used.
// Fortunately, the analyzers spawn a goroutine that does `actx.Store.Insert...`. 
// If Store is nil, it will panic in the goroutine. We can just test the detection methods directly for unit tests,
// or provide a nil-safe context if we modify the analyzers. 
// For now, we will test the internal detection methods directly to verify the pure logic.

func TestHallucination_FabricatedAction(t *testing.T) {
	d := NewHallucinationDetector()

	// Scenario 1: Model claims to run a tool, but no tools were actually called.
	turn := &types.Turn{
		ModelResponse: "I ran the command to check the files.",
		ToolCalls:     []types.ToolCall{},
	}
	
	sig := d.detectFabricatedAction(turn)
	if sig == nil {
		t.Errorf("Expected hallucination signal for fabricated action, got nil")
	} else if sig.Type != types.HallucinationFabricatedAction {
		t.Errorf("Expected type %v, got %v", types.HallucinationFabricatedAction, sig.Type)
	}

	// Scenario 2: Model ran a tool.
	turnWithTool := &types.Turn{
		ModelResponse: "I ran the command.",
		ToolCalls: []types.ToolCall{
			{ToolName: "bash"},
		},
	}
	if d.detectFabricatedAction(turnWithTool) != nil {
		t.Errorf("Expected nil when tools are actually present")
	}
}

func TestHallucination_FalseConfidence(t *testing.T) {
	d := NewHallucinationDetector()

	turn := &types.Turn{
		ModelResponse: "I am absolutely sure this worked.",
		ToolCalls: []types.ToolCall{
			{ToolName: "bash", Status: types.StatusError},
		},
	}
	
	sig := d.detectFalseConfidence(turn)
	if sig == nil {
		t.Errorf("Expected false confidence signal")
	}
}

func TestHallucination_DelusionalSuccess(t *testing.T) {
	d := NewHallucinationDetector()

	turn := &types.Turn{
		ModelResponse: "I have successfully fixed the issue. Everything is working.",
		ToolCalls: []types.ToolCall{
			{
				ToolName: "bash",
				Result:   []byte("Traceback (most recent call last):\n  File \"script.py\", line 1\nSyntaxError: invalid syntax"),
			},
		},
	}
	
	sig := d.detectDelusionalSuccess(turn)
	if sig == nil {
		t.Errorf("Expected delusional success signal when tool returns traceback and model claims success")
	}

	// Negative case
	turnSuccess := &types.Turn{
		ModelResponse: "I have successfully fixed the issue.",
		ToolCalls: []types.ToolCall{
			{
				ToolName: "bash",
				Result:   []byte("All tests passed."),
			},
		},
	}
	
	if d.detectDelusionalSuccess(turnSuccess) != nil {
		t.Errorf("Expected nil when tool result is actually successful")
	}
}
