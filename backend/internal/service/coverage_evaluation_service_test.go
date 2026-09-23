package service

import (
	"context"
	"testing"
	"time"

	"hazop-safeguard-coverage/backend/internal/algorithm"
	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
	"hazop-safeguard-coverage/backend/internal/repository"
	"hazop-safeguard-coverage/backend/internal/util"
)

func TestCoverageEvaluationRespectsSafeguardOutages(t *testing.T) {
	db := testDB(t)
	nodeRepo := repository.NewProcessNodeRepository(db)
	scenarioRepo := repository.NewDeviationScenarioRepository(db)
	safeguardRepo := repository.NewSafeguardRepository(db)
	outageRepo := repository.NewSafeguardOutageRepository(db)
	evaluationRepo := repository.NewCoverageEvaluationRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	ctx := context.Background()
	reviewer := util.Actor{UserID: 20, Username: "reviewer", Role: "safety_reviewer", RequestID: "coverage-outage-test"}

	node := model.ProcessNode{
		NodeCode: "T-301", Name: "Coverage Node", UnitName: "Test Unit", Medium: "water",
		DesignPressure: 1, DesignTemperature: 100, OwnerTeam: "test", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := nodeRepo.Create(ctx, &node); err != nil {
		t.Fatalf("create node: %v", err)
	}
	scenario := model.DeviationScenario{
		ProcessNodeID: node.ID, Guideword: "more", Parameter: "temperature",
		Cause: "cooling loss", Consequence: "overpressure", Likelihood: 4, Severity: 5,
		ScenarioState: "analyzed", Version: 1, CreatedBy: 10, CreatedByName: "engineer",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := scenarioRepo.Create(ctx, &scenario); err != nil {
		t.Fatalf("create scenario: %v", err)
	}
	safeguardService := NewSafeguardService(safeguardRepo, scenarioRepo, auditRepo)
	verifiedAt := time.Now().UTC().AddDate(0, 0, -10)
	safeguard, err := safeguardService.Create(ctx, dto.CreateSafeguardRequest{
		Name: "SIS trip", SafeguardType: "interlock", TargetScenarioID: scenario.ID,
		IndependenceKey: "SIS-T301", Effectiveness: 0.8, TestIntervalDays: 365,
		LastVerifiedAt: &verifiedAt, EvidenceNote: "proof test certificate",
	}, reviewer)
	if err != nil {
		t.Fatalf("create safeguard: %v", err)
	}

	evaluations := NewCoverageEvaluationService(
		evaluationRepo, scenarioRepo, nodeRepo, safeguardRepo, outageRepo, auditRepo, algorithm.NewEvaluator(),
	)
	outages := NewSafeguardOutageService(safeguardRepo, outageRepo, auditRepo)
	run := func(key string) dto.CoverageEvaluationResponse {
		t.Helper()
		result, _, err := evaluations.Run(ctx, dto.RunCoverageEvaluationRequest{ScenarioID: scenario.ID}, key, reviewer)
		if err != nil {
			t.Fatalf("run evaluation %s: %v", key, err)
		}
		return result
	}

	if result := run("baseline-0001"); result.CoverageScore != 80 {
		t.Fatalf("baseline coverage = %v, want 80", result.CoverageScore)
	}

	// A snapshot frozen inside the outage window excludes the safeguard.
	now := time.Now().UTC()
	outage, err := outages.Register(ctx, safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "planned bypass for proof test", StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, reviewer)
	if err != nil {
		t.Fatalf("register outage: %v", err)
	}
	if outage.Status != model.OutageActive {
		t.Fatalf("outage status = %q, want active", outage.Status)
	}
	during := run("during-outage-1")
	if during.CoverageScore != 0 {
		t.Fatalf("coverage during outage = %v, want 0", during.CoverageScore)
	}
	if len(during.DeduplicatedSafeguards) != 0 {
		t.Fatalf("outaged safeguard joined dedup: %#v", during.DeduplicatedSafeguards)
	}

	// Revoking the registration re-includes the safeguard immediately.
	if _, err = outages.Revoke(ctx, outage.ID, dto.RevokeSafeguardOutageRequest{Reason: "test cancelled"}, reviewer); err != nil {
		t.Fatalf("revoke outage: %v", err)
	}
	if result := run("after-revoke-01"); result.CoverageScore != 80 {
		t.Fatalf("coverage after revoke = %v, want 80", result.CoverageScore)
	}

	// A window that already ended does not affect new snapshots: the safeguard
	// is counted again without any manual reactivation.
	if _, err = outages.Register(ctx, safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "completed maintenance", StartsAt: now.Add(-48 * time.Hour), EndsAt: now.Add(-24 * time.Hour),
	}, reviewer); err != nil {
		t.Fatalf("register ended outage: %v", err)
	}
	if result := run("after-ended-001"); result.CoverageScore != 80 {
		t.Fatalf("coverage after ended outage = %v, want 80", result.CoverageScore)
	}
}
