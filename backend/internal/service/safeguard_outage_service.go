package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
	"hazop-safeguard-coverage/backend/internal/repository"
	"hazop-safeguard-coverage/backend/internal/util"
	"net/http"
	"strconv"
	"time"
)

type SafeguardOutageService interface {
	Register(context.Context, uint, dto.RegisterSafeguardOutageRequest, util.Actor) (dto.SafeguardOutageResponse, error)
	Get(context.Context, uint) (dto.SafeguardOutageResponse, error)
	List(context.Context, dto.SafeguardOutageQuery) (dto.SafeguardOutageListResponse, error)
	Revoke(context.Context, uint, dto.RevokeSafeguardOutageRequest, util.Actor) (dto.SafeguardOutageResponse, error)
}

type safeguardOutageService struct {
	outages    repository.SafeguardOutageRepository
	safeguards repository.SafeguardRepository
	audits     repository.AuditRepository
	db         *gorm.DB
	now        func() time.Time
}

func NewSafeguardOutageService(
	outages repository.SafeguardOutageRepository,
	safeguards repository.SafeguardRepository,
	audits repository.AuditRepository,
	db *gorm.DB,
) SafeguardOutageService {
	return &safeguardOutageService{
		outages: outages, safeguards: safeguards, audits: audits, db: db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *safeguardOutageService) Register(
	ctx context.Context,
	safeguardID uint,
	request dto.RegisterSafeguardOutageRequest,
	actor util.Actor,
) (dto.SafeguardOutageResponse, error) {
	request.Normalize()
	now := s.now().Truncate(time.Second)
	if request.EndsAt.Before(request.StartsAt) || request.EndsAt.Equal(request.StartsAt) {
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "ends_at must be later than starts_at")
	}
	// The window may already be open (maintenance started just before the
	// reviewer records it), but it must not already be over: an elapsed window
	// cannot exclude a safeguard from a fresh evaluation.
	if !request.EndsAt.After(now) {
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "ends_at must be in the future")
	}
	if request.StartsAt.Before(now.AddDate(0, 0, -7)) {
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "starts_at is more than seven days in the past")
	}
	if _, err := s.safeguards.GetByID(ctx, safeguardID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardOutageResponse{}, util.NotFound("safeguard")
		}
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard", err)
	}
	// Planned windows cannot start too far in the future, so maintenance
	// planning stays reviewable while accidental year-2100 entries are rejected.
	if request.StartsAt.After(now.AddDate(2, 0, 0)) {
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "starts_at is more than two years away")
	}

	var response dto.SafeguardOutageResponse
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Re-read inside the transaction so concurrent registrations cannot both
		// pass the overlap check. SQLite runs with a single connection, which
		// serializes writers; Postgres relies on the transaction plus the
		// safeguard/window index for a stable ordering.
		existing, listErr := s.outages.ListEffectiveBySafeguard(ctx, tx, safeguardID)
		if listErr != nil {
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to check existing outage windows", listErr)
		}
		for _, window := range existing {
			if window.Overlaps(request.StartsAt, request.EndsAt) {
				return util.NewError(
					http.StatusConflict, util.CodeConflict,
					"outage window overlaps an existing registration #"+strconv.Itoa(int(window.ID))+
						" ("+window.StartsAt.Format(time.RFC3339)+" to "+window.EndsAt.Format(time.RFC3339)+")",
				)
			}
		}
		outage := model.SafeguardOutage{
			SafeguardID: safeguardID, Reason: request.Reason,
			StartsAt: request.StartsAt, EndsAt: request.EndsAt,
			RegisteredBy: actor.UserID, RegisteredByName: actor.Username,
			CreatedAt: now, UpdatedAt: now,
		}
		if createErr := s.outages.Create(ctx, tx, &outage); createErr != nil {
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to register safeguard outage", createErr)
		}
		if auditErr := s.recordAuditTx(ctx, tx, actor, outage, "outage_register", nil, outage); auditErr != nil {
			return auditErr
		}
		response = dto.NewSafeguardOutageResponse(outage, now)
		return nil
	})
	if err != nil {
		return dto.SafeguardOutageResponse{}, err
	}
	return response, nil
}

func (s *safeguardOutageService) Get(ctx context.Context, id uint) (dto.SafeguardOutageResponse, error) {
	outage, err := s.outages.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardOutageResponse{}, util.NotFound("safeguard outage")
		}
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard outage", err)
	}
	return dto.NewSafeguardOutageResponse(outage, s.now()), nil
}

func (s *safeguardOutageService) List(
	ctx context.Context,
	query dto.SafeguardOutageQuery,
) (dto.SafeguardOutageListResponse, error) {
	now := s.now()
	outages, total, err := s.outages.List(ctx, query, now)
	if err != nil {
		return dto.SafeguardOutageListResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to list safeguard outages", err)
	}
	response := dto.SafeguardOutageListResponse{
		Items: make([]dto.SafeguardOutageResponse, 0, len(outages)),
		Total: total, Page: query.Page, Size: query.PageSize,
	}
	for _, outage := range outages {
		response.Items = append(response.Items, dto.NewSafeguardOutageResponse(outage, now))
	}
	return response, nil
}

func (s *safeguardOutageService) Revoke(
	ctx context.Context,
	id uint,
	request dto.RevokeSafeguardOutageRequest,
	actor util.Actor,
) (dto.SafeguardOutageResponse, error) {
	request.Normalize()
	before, err := s.outages.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardOutageResponse{}, util.NotFound("safeguard outage")
		}
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard outage", err)
	}
	if before.RevokedAt != nil {
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusConflict, util.CodeStateTransition, "outage registration was already revoked")
	}
	now := s.now()
	changed, err := s.outages.Revoke(ctx, id, map[string]any{
		"revoked_at": now.UTC(), "revoked_by": actor.UserID,
		"revoked_by_name": actor.Username, "revoke_reason": request.Reason,
	})
	if err != nil {
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to revoke safeguard outage", err)
	}
	if !changed {
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusConflict, util.CodeStateTransition, "outage registration was already revoked")
	}
	after, err := s.outages.GetByID(ctx, id)
	if err != nil {
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to reload safeguard outage", err)
	}
	if err := s.recordAudit(ctx, actor, after, "outage_revoke", before, after); err != nil {
		return dto.SafeguardOutageResponse{}, err
	}
	return dto.NewSafeguardOutageResponse(after, now), nil
}

func (s *safeguardOutageService) recordAudit(
	ctx context.Context,
	actor util.Actor,
	outage model.SafeguardOutage,
	action string,
	before any,
	after any,
) error {
	return s.recordAuditTx(ctx, s.db, actor, outage, action, before, after)
}

func (s *safeguardOutageService) recordAuditTx(
	ctx context.Context,
	conn *gorm.DB,
	actor util.Actor,
	outage model.SafeguardOutage,
	action string,
	before any,
	after any,
) error {
	beforeJSON, err := snapshotJSON(before)
	if err != nil {
		return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to serialize audit snapshot", err)
	}
	afterJSON, err := snapshotJSON(after)
	if err != nil {
		return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to serialize audit snapshot", err)
	}
	log := model.AuditLog{
		RequestID: actor.RequestID, ActorID: actor.UserID, ActorName: actor.Username,
		ActorRole: actor.Role, EntityType: "safeguard_outage", EntityID: outage.ID, Action: action,
		BeforeSnapshot: beforeJSON, AfterSnapshot: afterJSON,
		ResultSummary: "safeguard " + strconv.Itoa(int(outage.SafeguardID)) + " outage " +
			outage.StartsAt.Format(time.RFC3339) + ".." + outage.EndsAt.Format(time.RFC3339),
		CreatedAt: s.now(),
	}
	if err := s.audits.RecordTx(ctx, conn, log); err != nil {
		return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to record outage audit", err)
	}
	return nil
}
