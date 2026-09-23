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
	first, err := evaluator.Evaluate(NewSnapshot(node, scenario, safeguards, reference))
	if err != nil {
		t.Fatalf("first evaluation failed: %v", err)
	}
	second, err := evaluator.Evaluate(NewSnapshot(node, scenario, safeguards, reference))
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
	result, err := NewEvaluator().Evaluate(NewSnapshot(node, scenario, nil, reference))
	if err != nil {
		t.Fatalf("evaluation failed: %v", err)
	}
	if result.CoverageScore != 0 || len(result.Explanation.Paths) != 1 || result.Explanation.Paths[0].Covered {
		t.Fatalf("expected one unprotected path, got %#v", result.Explanation.Paths)
	}
}

func TestEvaluatorExcludesSafeguardsInPlannedOutage(t *testing.T) {
	t.Parallel()
	reference := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	valid := reference.AddDate(0, 0, -10)
	node := model.ProcessNode{ID: 7, NodeCode: "R-7", Name: "Reactor"}
	scenario := model.DeviationScenario{
		ID: 9, Guideword: "more", Parameter: "temperature",
		Cause: "cooling loss", Consequence: "overpressure",
		Likelihood: 4, Severity: 5, ScenarioState: "analyzed", Version: 2,
	}
	safeguards := []model.Safeguard{
		{ID: 2, IndependenceKey: "SIS-A", Effectiveness: 0.8, TestIntervalDays: 365, LastVerifiedAt: &valid, LifecycleState: "active"},
		{ID: 3, IndependenceKey: "SIS-A", Effectiveness: 0.4, TestIntervalDays: 365, LastVerifiedAt: &valid, LifecycleState: "active"},
	}
	evaluator := NewEvaluator()

	baseline, err := evaluator.Evaluate(NewSnapshot(node, scenario, safeguards, reference))
	if err != nil {
		t.Fatalf("baseline evaluation failed: %v", err)
	}
	if baseline.CoverageScore != 80 || len(baseline.Explanation.Deduplicated) != 1 {
		t.Fatalf("unexpected baseline: score=%v dedup=%#v", baseline.CoverageScore, baseline.Explanation.Deduplicated)
	}

	// Safeguard 2 is in a registered outage at the freeze time: it must join
	// neither the independence dedup nor the coverage score, so safeguard 3
	// becomes the retained layer for the shared key.
	outaged, err := evaluator.Evaluate(NewSnapshotWithOutages(node, scenario, safeguards, map[uint]uint{2: 11}, reference))
	if err != nil {
		t.Fatalf("outage evaluation failed: %v", err)
	}
	if outaged.CoverageScore != 40 {
		t.Fatalf("coverage score = %v, want 40 (outaged layer excluded)", outaged.CoverageScore)
	}
	if len(outaged.Explanation.Deduplicated) != 0 {
		t.Fatalf("outaged safeguard took part in deduplication: %#v", outaged.Explanation.Deduplicated)
	}
	foundRejection := false
	for _, step := range outaged.Explanation.ScoreSteps {
		if step.Rule == "eligibility-filter" && step.Input == "safeguard=2" {
			foundRejection = true
		}
	}
	if !foundRejection {
		t.Fatalf("missing eligibility-filter step for outaged safeguard: %#v", outaged.Explanation.ScoreSteps)
	}

	// The outage marker lives inside the frozen snapshot, so replay is exact.
	passed, replayed, err := evaluator.Replay(outaged.SnapshotJSON, outaged.InputHash, outaged.CoverageScore)
	if err != nil || !passed || replayed.CoverageScore != outaged.CoverageScore {
		t.Fatalf("replay of outage snapshot failed: passed=%t err=%v", passed, err)
	}
}
