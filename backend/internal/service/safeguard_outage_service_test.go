package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
	"hazop-safeguard-coverage/backend/internal/repository"
	"hazop-safeguard-coverage/backend/internal/util"
)

func TestSafeguardOutageRegistrationRules(t *testing.T) {
	db := testDB(t)
	safeguardRepo := repository.NewSafeguardRepository(db)
	outageRepo := repository.NewSafeguardOutageRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	scenarioRepo := repository.NewDeviationScenarioRepository(db)
	nodeRepo := repository.NewProcessNodeRepository(db)
	ctx := context.Background()
	reviewer := util.Actor{UserID: 20, Username: "reviewer", Role: "safety_reviewer", RequestID: "outage-test"}

	node := model.ProcessNode{
		NodeCode: "T-201", Name: "Outage Node", UnitName: "Test Unit", Medium: "water",
		DesignPressure: 1, DesignTemperature: 100, OwnerTeam: "test", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := nodeRepo.Create(ctx, &node); err != nil {
		t.Fatalf("create node: %v", err)
	}
	scenario := model.DeviationScenario{
		ProcessNodeID: node.ID, Guideword: "more", Parameter: "pressure",
		Cause: "blocked outlet", Consequence: "rupture", Likelihood: 3, Severity: 5,
		ScenarioState: "draft", Version: 1, CreatedBy: 10, CreatedByName: "engineer",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := scenarioRepo.Create(ctx, &scenario); err != nil {
		t.Fatalf("create scenario: %v", err)
	}
	safeguardService := NewSafeguardService(safeguardRepo, scenarioRepo, auditRepo)
	safeguard, err := safeguardService.Create(ctx, dto.CreateSafeguardRequest{
		Name: "Trip valve", SafeguardType: "interlock", TargetScenarioID: scenario.ID,
		IndependenceKey: "SIS-T201", Effectiveness: 0.8, TestIntervalDays: 365,
		EvidenceNote: "proof test certificate",
	}, reviewer)
	if err != nil {
		t.Fatalf("create safeguard: %v", err)
	}

	service := NewSafeguardOutageService(safeguardRepo, outageRepo, auditRepo)
	now := time.Now().UTC()
	window := dto.RegisterSafeguardOutageRequest{
		Reason:   "planned proof test",
		StartsAt: now.Add(24 * time.Hour),
		EndsAt:   now.Add(48 * time.Hour),
	}

	// Invalid window is rejected before any persistence.
	invalid := window
	invalid.EndsAt = invalid.StartsAt
	if _, err := service.Register(ctx, safeguard.ID, invalid, reviewer); err == nil {
		t.Fatal("expected validation error for empty window")
	} else {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Status != 422 {
			t.Fatalf("empty window should return 422, got %v", err)
		}
	}

	first, err := service.Register(ctx, safeguard.ID, window, reviewer)
	if err != nil {
		t.Fatalf("register outage: %v", err)
	}
	if first.Status != model.OutageScheduled {
		t.Fatalf("future window status = %q, want scheduled", first.Status)
	}

	// A later submission overlapping the registered window cannot take effect.
	overlapping := window
	overlapping.StartsAt = window.StartsAt.Add(12 * time.Hour)
	overlapping.EndsAt = window.EndsAt.Add(12 * time.Hour)
	if _, err = service.Register(ctx, safeguard.ID, overlapping, reviewer); err == nil {
		t.Fatal("overlapping registration should be rejected")
	} else {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Status != 409 {
			t.Fatalf("overlapping registration should return 409, got %v", err)
		}
	}

	// A window merely touching the registered one is not an overlap.
	touching := window
	touching.StartsAt = window.EndsAt
	touching.EndsAt = window.EndsAt.Add(24 * time.Hour)
	if _, err = service.Register(ctx, safeguard.ID, touching, reviewer); err != nil {
		t.Fatalf("adjacent window should be accepted: %v", err)
	}

	// Revoking keeps the record, marks it revoked, and frees the window.
	revoked, err := service.Revoke(ctx, first.ID, dto.RevokeSafeguardOutageRequest{Reason: "test rescheduled"}, reviewer)
	if err != nil {
		t.Fatalf("revoke outage: %v", err)
	}
	if revoked.Status != model.OutageRevoked || revoked.RevokedBy == nil || revoked.RevokeReason != "test rescheduled" {
		t.Fatalf("unexpected revoked record: %#v", revoked)
	}
	if _, err = service.Revoke(ctx, first.ID, dto.RevokeSafeguardOutageRequest{Reason: "again"}, reviewer); err == nil {
		t.Fatal("revoking an already revoked record should fail")
	}
	if _, err = service.Register(ctx, safeguard.ID, window, reviewer); err != nil {
		t.Fatalf("window freed by revocation should accept new registration: %v", err)
	}

	// An ended window stays in the ledger and cannot be revoked.
	ended, err := service.Register(ctx, safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "historical record", StartsAt: now.Add(-72 * time.Hour), EndsAt: now.Add(-48 * time.Hour),
	}, reviewer)
	if err != nil {
		t.Fatalf("register ended outage: %v", err)
	}
	if ended.Status != model.OutageEnded {
		t.Fatalf("past window status = %q, want ended", ended.Status)
	}
	if _, err = service.Revoke(ctx, ended.ID, dto.RevokeSafeguardOutageRequest{Reason: "too late"}, reviewer); err == nil {
		t.Fatal("revoking an ended outage should fail")
	}

	// The ledger exposes every registration with its derived status.
	ledger, err := service.List(ctx, dto.SafeguardOutageQuery{SafeguardID: safeguard.ID, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list outages: %v", err)
	}
	if ledger.Total != 4 {
		t.Fatalf("ledger total = %d, want 4", ledger.Total)
	}
	statuses := map[string]int{}
	for _, item := range ledger.Items {
		statuses[item.Status]++
	}
	if statuses[model.OutageScheduled] != 2 || statuses[model.OutageRevoked] != 1 || statuses[model.OutageEnded] != 1 {
		t.Fatalf("unexpected ledger statuses: %#v", statuses)
	}
	active, err := service.List(ctx, dto.SafeguardOutageQuery{SafeguardID: safeguard.ID, Status: model.OutageActive, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list active outages: %v", err)
	}
	if active.Total != 0 {
		t.Fatalf("no window covers now, active total = %d, want 0", active.Total)
	}
}
