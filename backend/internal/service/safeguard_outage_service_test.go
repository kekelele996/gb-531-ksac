package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"hazop-safeguard-coverage/backend/internal/algorithm"
	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
	"hazop-safeguard-coverage/backend/internal/repository"
	"hazop-safeguard-coverage/backend/internal/util"
	"strings"

	"gorm.io/gorm"
)

func outageFixture(t *testing.T) (SafeguardOutageService, SafeguardService, *model.Safeguard, *gorm.DB) {
	t.Helper()
	db := testDB(t)
	safeguardRepo := repository.NewSafeguardRepository(db)
	scenarioRepo := repository.NewDeviationScenarioRepository(db)
	outageRepo := repository.NewSafeguardOutageRepository(db)
	auditRepo := repository.NewAuditRepository(db)

	node := model.ProcessNode{
		NodeCode: "T-202", Name: "Outage Node", UnitName: "Test Unit", Medium: "water",
		DesignPressure: 1, DesignTemperature: 100, OwnerTeam: "test", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	scenario := model.DeviationScenario{
		ProcessNodeID: node.ID, Guideword: "more", Parameter: "pressure",
		Cause: "blocked outlet", Consequence: "rupture", Likelihood: 3, Severity: 5,
		ScenarioState: "analyzed", Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&scenario).Error; err != nil {
		t.Fatalf("create scenario: %v", err)
	}
	verified := time.Now().UTC().AddDate(0, 0, -10)
	safeguard := model.Safeguard{
		Name: "Trip switch", SafeguardType: "interlock", TargetScenarioID: scenario.ID,
		IndependenceKey: "SIS-T-202", Effectiveness: 0.9, TestIntervalDays: 365,
		LastVerifiedAt: &verified, LifecycleState: "active", EvidenceNote: "cert",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&safeguard).Error; err != nil {
		t.Fatalf("create safeguard: %v", err)
	}
	outageService := NewSafeguardOutageService(outageRepo, safeguardRepo, auditRepo, db)
	safeguardService := NewSafeguardService(safeguardRepo, scenarioRepo, auditRepo)
	return outageService, safeguardService, &safeguard, db
}

func TestOutageRegistrationLifecycleStates(t *testing.T) {
	svc, _, safeguard, db := outageFixture(t)
	actor := util.Actor{UserID: 20, Username: "reviewer", Role: "safety_reviewer", RequestID: "req-outage-1"}
	now := time.Now().UTC().Truncate(time.Second)

	pending, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "future proof test", StartsAt: now.Add(48 * time.Hour), EndsAt: now.Add(72 * time.Hour),
	}, actor)
	if err != nil {
		t.Fatalf("register future outage: %v", err)
	}
	if pending.Status != "pending" {
		t.Fatalf("future window status = %q, want pending", pending.Status)
	}

	active, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "live maintenance window", StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, actor)
	if err != nil {
		t.Fatalf("register active outage: %v", err)
	}
	if active.Status != "active" {
		t.Fatalf("live window status = %q, want active", active.Status)
	}

	list, err := svc.List(context.Background(), dto.SafeguardOutageQuery{SafeguardID: safeguard.ID, Page: 1, PageSize: 20})
	if err != nil || list.Total != 2 {
		t.Fatalf("list outages: total=%d err=%v", list.Total, err)
	}

	// A window whose end time has already elapsed cannot be registered: it could
	// never exclude a safeguard from a new evaluation.
	if _, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "already finished window", StartsAt: now.Add(-4 * time.Hour), EndsAt: now.Add(-2 * time.Hour),
	}, actor); err == nil {
		t.Fatal("elapsed window registration should be rejected")
	}

	// Natural expiry needs no manual action: a non-revoked window whose end has
	// passed is reported as ended.
	storedEnded := model.SafeguardOutage{
		SafeguardID: safeguard.ID, Reason: "naturally elapsed window",
		StartsAt: now.Add(-4 * time.Hour), EndsAt: now.Add(-2 * time.Hour),
		RegisteredBy: actor.UserID, RegisteredByName: actor.Username, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&storedEnded).Error; err != nil {
		t.Fatalf("insert elapsed window: %v", err)
	}
	got, err := svc.Get(context.Background(), storedEnded.ID)
	if err != nil || got.Status != "ended" {
		t.Fatalf("elapsed window status = %q err=%v, want ended", got.Status, err)
	}
}

func TestOverlappingWindowsAreRejected(t *testing.T) {
	svc, _, safeguard, _ := outageFixture(t)
	actor := util.Actor{UserID: 20, Username: "reviewer", Role: "safety_reviewer", RequestID: "req-outage-2"}
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "first window", StartsAt: now.Add(time.Hour), EndsAt: now.Add(5 * time.Hour),
	}, actor); err != nil {
		t.Fatalf("register first window: %v", err)
	}

	cases := []struct {
		name  string
		start time.Duration
		end   time.Duration
	}{
		{"contained window", 2 * time.Hour, 3 * time.Hour},
		{"overlapping start", -1 * time.Hour, 2 * time.Hour},
		{"overlapping end", 4 * time.Hour, 6 * time.Hour},
		{"enclosing window", -2 * time.Hour, 8 * time.Hour},
	}
	for _, tc := range cases {
		_, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
			Reason: tc.name, StartsAt: now.Add(tc.start), EndsAt: now.Add(tc.end),
		}, actor)
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Status != 409 {
			t.Fatalf("%s: expected 409 conflict, got %v", tc.name, err)
		}
	}

	// Touching windows are allowed: a new window may start exactly at the end.
	if _, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "back to back window", StartsAt: now.Add(5 * time.Hour), EndsAt: now.Add(9 * time.Hour),
	}, actor); err != nil {
		t.Fatalf("back-to-back window should be accepted: %v", err)
	}
}

func TestRevokeEndsWindowAndKeepsRegistration(t *testing.T) {
	svc, _, safeguard, _ := outageFixture(t)
	actor := util.Actor{UserID: 20, Username: "reviewer", Role: "safety_reviewer", RequestID: "req-outage-3"}
	now := time.Now().UTC().Truncate(time.Second)

	registered, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "will be cancelled", StartsAt: now.Add(time.Hour), EndsAt: now.Add(5 * time.Hour),
	}, actor)
	if err != nil {
		t.Fatalf("register window: %v", err)
	}
	revoked, err := svc.Revoke(context.Background(), registered.ID, dto.RevokeSafeguardOutageRequest{
		Reason: "maintenance rescheduled",
	}, actor)
	if err != nil {
		t.Fatalf("revoke window: %v", err)
	}
	if revoked.Status != "ended" || revoked.RevokedAt == nil || revoked.RevokeReason == "" {
		t.Fatalf("revoked response = %#v", revoked)
	}
	if _, err := svc.Revoke(context.Background(), registered.ID, dto.RevokeSafeguardOutageRequest{
		Reason: "second attempt",
	}, actor); err == nil {
		t.Fatal("second revoke should be rejected")
	}

	// After revocation the same slot can be registered again because the revoked
	// window no longer participates in overlap checks.
	if _, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "replacement window", StartsAt: now.Add(2 * time.Hour), EndsAt: now.Add(3 * time.Hour),
	}, actor); err != nil {
		t.Fatalf("replacement window after revoke should be accepted: %v", err)
	}
}

func TestInvalidWindowsAreRejected(t *testing.T) {
	svc, _, safeguard, _ := outageFixture(t)
	actor := util.Actor{UserID: 20, Username: "reviewer", Role: "safety_reviewer", RequestID: "req-outage-4"}
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "inverted window", StartsAt: now.Add(3 * time.Hour), EndsAt: now.Add(time.Hour),
	}, actor); err == nil {
		t.Fatal("end before start should be rejected")
	}
	if _, err := svc.Register(context.Background(), safeguard.ID, dto.RegisterSafeguardOutageRequest{
		Reason: "window starting long ago", StartsAt: now.AddDate(0, 0, -8), EndsAt: now.Add(time.Hour),
	}, actor); err == nil {
		t.Fatal("window starting more than seven days ago should be rejected")
	}
	if _, err := svc.Register(context.Background(), 99999, dto.RegisterSafeguardOutageRequest{
		Reason: "missing safeguard", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour),
	}, actor); err == nil {
		t.Fatal("unknown safeguard should be rejected")
	}
}

func TestOutageExcludesSafeguardFromCoverageAndRestoresAfterWindow(t *testing.T) {
	db := testDB(t)
	nodeRepo := repository.NewProcessNodeRepository(db)
	scenarioRepo := repository.NewDeviationScenarioRepository(db)
	safeguardRepo := repository.NewSafeguardRepository(db)
	outageRepo := repository.NewSafeguardOutageRepository(db)
	evaluationRepo := repository.NewCoverageEvaluationRepository(db)
	auditRepo := repository.NewAuditRepository(db)

	node := model.ProcessNode{
		NodeCode: "R-9", Name: "Outage Reactor", UnitName: "unit", Medium: "gas",
		DesignPressure: 2, DesignTemperature: 200, OwnerTeam: "team", Status: "active",
	}
	if err := nodeRepo.Create(context.Background(), &node); err != nil {
		t.Fatalf("create node: %v", err)
	}
	scenario := model.DeviationScenario{
		ProcessNodeID: node.ID, Guideword: "more", Parameter: "temperature",
		Cause: "cooling loss", Consequence: "overpressure", Likelihood: 4, Severity: 5,
		ScenarioState: "analyzed", Version: 1,
	}
	if err := scenarioRepo.Create(context.Background(), &scenario); err != nil {
		t.Fatalf("create scenario: %v", err)
	}
	verified := time.Now().UTC().AddDate(0, 0, -10)
	safeguards := []model.Safeguard{
		{
			Name: "SIS trip", SafeguardType: "interlock", TargetScenarioID: scenario.ID,
			IndependenceKey: "SIS-A", Effectiveness: 0.8, TestIntervalDays: 365,
			LastVerifiedAt: &verified, LifecycleState: "active", EvidenceNote: "cert-a",
		},
		{
			Name: "Relief valve", SafeguardType: "relief", TargetScenarioID: scenario.ID,
			IndependenceKey: "PSV-B", Effectiveness: 0.5, TestIntervalDays: 365,
			LastVerifiedAt: &verified, LifecycleState: "active", EvidenceNote: "cert-b",
		},
	}
	for i := range safeguards {
		if err := safeguardRepo.Create(context.Background(), &safeguards[i]); err != nil {
			t.Fatalf("create safeguard: %v", err)
		}
	}

	svc := NewCoverageEvaluationService(
		evaluationRepo, scenarioRepo, nodeRepo, safeguardRepo, outageRepo, auditRepo,
		algorithm.NewEvaluator(),
	)
	actor := util.Actor{UserID: 30, Username: "engineer", Role: "process_engineer", RequestID: "req-eval-1"}

	// Baseline: both layers combine to 1 - 0.2*0.5 = 0.9 -> 90.
	baseline, existing, err := svc.Run(context.Background(), dto.RunCoverageEvaluationRequest{ScenarioID: scenario.ID}, "key-baseline-0001", actor)
	if err != nil || existing {
		t.Fatalf("baseline run: existing=%t err=%v", existing, err)
	}
	if baseline.CoverageScore != 90 {
		t.Fatalf("baseline score = %v, want 90", baseline.CoverageScore)
	}

	// Freeze an outage window for the SIS trip that starts in the past and ends
	// later today, then run again with a fresh idempotency key.
	now := time.Now().UTC().Truncate(time.Second)
	window := model.SafeguardOutage{
		SafeguardID: safeguards[0].ID, Reason: "annual proof test bypass",
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
		RegisteredBy: 20, RegisteredByName: "reviewer", CreatedAt: now, UpdatedAt: now,
	}
	if err := outageRepo.Create(context.Background(), nil, &window); err != nil {
		t.Fatalf("create outage: %v", err)
	}
	during, existing, err := svc.Run(context.Background(), dto.RunCoverageEvaluationRequest{ScenarioID: scenario.ID}, "key-outage-000001", actor)
	if err != nil || existing {
		t.Fatalf("outage run: existing=%t err=%v", existing, err)
	}
	// Only the 0.5 relief valve remains: score 50; the SIS trip must not be
	// independence-deduplicated nor counted.
	if during.CoverageScore != 50 {
		t.Fatalf("outage score = %v, want 50", during.CoverageScore)
	}
	foundOutageRejection := false
	for _, step := range during.Explanation.ScoreSteps {
		if step.Rule == "eligibility-filter" && strings.Contains(step.Explanation, "planned maintenance") {
			foundOutageRejection = true
		}
	}
	if !foundOutageRejection {
		t.Fatalf("outage rejection missing from score steps: %#v", during.Explanation.ScoreSteps)
	}

	// Revoke the window: no manual safeguard restore is needed, the layer must
	// be counted again on the next run.
	if _, err := NewSafeguardOutageService(outageRepo, safeguardRepo, auditRepo, db).Revoke(
		context.Background(), window.ID,
		dto.RevokeSafeguardOutageRequest{Reason: "proof test finished early"},
		util.Actor{UserID: 20, Username: "reviewer", Role: "safety_reviewer", RequestID: "req-revoke"},
	); err != nil {
		t.Fatalf("revoke outage: %v", err)
	}
	restored, existing, err := svc.Run(context.Background(), dto.RunCoverageEvaluationRequest{ScenarioID: scenario.ID}, "key-restored-0001", actor)
	if err != nil || existing {
		t.Fatalf("restored run: existing=%t err=%v", existing, err)
	}
	if restored.CoverageScore != 90 {
		t.Fatalf("restored score = %v, want 90", restored.CoverageScore)
	}

	// The historical evaluation must still carry the frozen outage in its
	// immutable input snapshot.
	var snapshot struct {
		Safeguards []struct {
			ID     uint `json:"id"`
			Outage *struct {
				Reason string `json:"reason"`
			} `json:"outage"`
		} `json:"safeguards"`
	}
	if err := json.Unmarshal(during.InputSnapshot, &snapshot); err != nil {
		t.Fatalf("decode frozen snapshot: %v", err)
	}
	frozen := false
	for _, item := range snapshot.Safeguards {
		if item.ID == safeguards[0].ID && item.Outage != nil && item.Outage.Reason != "" {
			frozen = true
		}
	}
	if !frozen {
		t.Fatalf("active outage was not frozen into evaluation snapshot: %s", string(during.InputSnapshot))
	}
}
