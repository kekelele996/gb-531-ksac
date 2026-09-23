package algorithm

import (
	"testing"
	"time"

	"hazop-safeguard-coverage/backend/internal/model"
)

func TestEvaluatorIsDeterministicAndDeduplicatesIndependenceKeys(t *testing.T) {
	t.Parallel()
	reference := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	valid := reference.AddDate(0, 0, -10)
	expired := reference.AddDate(0, 0, -400)
	node := model.ProcessNode{ID: 7, NodeCode: "R-7", Name: "Reactor"}
	scenario := model.DeviationScenario{
		ID: 9, Guideword: "more", Parameter: "temperature",
		Cause: "cooling loss; runaway reaction", Consequence: "overpressure; release",
		Likelihood: 4, Severity: 5, ScenarioState: "analyzed", Version: 2,
	}
	safeguards := []model.Safeguard{
		{ID: 3, IndependenceKey: "SIS-A", Effectiveness: 0.4, TestIntervalDays: 365, LastVerifiedAt: &valid, LifecycleState: "active"},
		{ID: 2, IndependenceKey: "SIS-A", Effectiveness: 0.8, TestIntervalDays: 365, LastVerifiedAt: &valid, LifecycleState: "active"},
		{ID: 4, IndependenceKey: "PSV-B", Effectiveness: 0.9, TestIntervalDays: 365, LastVerifiedAt: &expired, LifecycleState: "expired"},
	}
	evaluator := NewEvaluator()
	first, err := evaluator.Evaluate(NewSnapshot(node, scenario, safeguards, nil, reference))
	if err != nil {
		t.Fatalf("first evaluation failed: %v", err)
	}
	second, err := evaluator.Evaluate(NewSnapshot(node, scenario, safeguards, nil, reference))
	if err != nil {
		t.Fatalf("second evaluation failed: %v", err)
	}
	if first.InputHash != second.InputHash || first.ExplanationJSON != second.ExplanationJSON {
		t.Fatal("same frozen input produced different output")
	}
	if first.CoverageScore != 80 {
		t.Fatalf("coverage score = %v, want 80", first.CoverageScore)
	}
	if len(first.Explanation.Deduplicated) != 1 || first.Explanation.Deduplicated[0].KeptID != 2 {
		t.Fatalf("unexpected deduplication: %#v", first.Explanation.Deduplicated)
	}
	passed, replayed, err := evaluator.Replay(first.SnapshotJSON, first.InputHash, first.CoverageScore)
	if err != nil || !passed || replayed.CoverageScore != first.CoverageScore {
		t.Fatalf("replay failed: passed=%t score=%v err=%v", passed, replayed.CoverageScore, err)
	}
}

func TestEvaluatorDetectsUnprotectedPaths(t *testing.T) {
	t.Parallel()
	reference := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	node := model.ProcessNode{ID: 1, NodeCode: "V-1", Name: "Vessel"}
	scenario := model.DeviationScenario{
		ID: 1, Guideword: "more", Parameter: "pressure",
		Cause: "blocked outlet", Consequence: "rupture", Likelihood: 3, Severity: 5,
		ScenarioState: "draft", Version: 1,
	}
	result, err := NewEvaluator().Evaluate(NewSnapshot(node, scenario, nil, nil, reference))
	if err != nil {
		t.Fatalf("evaluation failed: %v", err)
	}
	if result.CoverageScore != 0 || len(result.Explanation.Paths) != 1 || result.Explanation.Paths[0].Covered {
		t.Fatalf("expected one unprotected path, got %#v", result.Explanation.Paths)
	}
}

func TestEvaluatorExcludesSafeguardUnderPlannedOutage(t *testing.T) {
	t.Parallel()
	reference := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	verified := reference.AddDate(0, 0, -10)
	node := model.ProcessNode{ID: 4, NodeCode: "R-4", Name: "Reactor"}
	scenario := model.DeviationScenario{
		ID: 11, Guideword: "more", Parameter: "pressure",
		Cause: "loss of cooling", Consequence: "rupture", Likelihood: 3, Severity: 5,
		ScenarioState: "analyzed", Version: 1,
	}
	safeguards := []model.Safeguard{
		{ID: 1, Name: "Bypassed trip", SafeguardType: "interlock", IndependenceKey: "SIS-A", Effectiveness: 0.9, TestIntervalDays: 365, LastVerifiedAt: &verified, LifecycleState: "active"},
		{ID: 2, Name: "Relief valve", SafeguardType: "relief", IndependenceKey: "PSV-B", Effectiveness: 0.5, TestIntervalDays: 365, LastVerifiedAt: &verified, LifecycleState: "active"},
	}
	outages := map[uint]model.SafeguardOutage{
		1: {
			ID: 77, SafeguardID: 1, Reason: "annual proof test",
			StartsAt: reference.Add(-time.Hour), EndsAt: reference.Add(time.Hour),
		},
	}
	result, err := NewEvaluator().Evaluate(NewSnapshot(node, scenario, safeguards, outages, reference))
	if err != nil {
		t.Fatalf("evaluation failed: %v", err)
	}
	// Only the 0.5 relief valve remains; the bypassed trip neither scores nor
	// occupies its independence key.
	if result.CoverageScore != 50 {
		t.Fatalf("coverage score = %v, want 50", result.CoverageScore)
	}
	if len(result.Explanation.Deduplicated) != 0 {
		t.Fatalf("out-of-service safeguard must not participate in dedup: %#v", result.Explanation.Deduplicated)
	}
	rejected := false
	for _, step := range result.Explanation.ScoreSteps {
		if step.Rule == "eligibility-filter" && step.Input == "safeguard=1" {
			rejected = true
		}
	}
	if !rejected {
		t.Fatalf("missing rejection step for safeguard under outage: %#v", result.Explanation.ScoreSteps)
	}

	// Replay must remain deterministic with the frozen outage embedded.
	passed, _, err := NewEvaluator().Replay(result.SnapshotJSON, result.InputHash, result.CoverageScore)
	if err != nil || !passed {
		t.Fatalf("replay failed: passed=%t err=%v", passed, err)
	}

	// Once the window is over, the same safeguards count both layers again.
	restored, err := NewEvaluator().Evaluate(NewSnapshot(node, scenario, safeguards, nil, reference.Add(2*time.Hour)))
	if err != nil {
		t.Fatalf("restored evaluation failed: %v", err)
	}
	if restored.CoverageScore != 95 {
		t.Fatalf("restored coverage score = %v, want 95", restored.CoverageScore)
	}
}
