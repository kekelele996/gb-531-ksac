package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"gorm.io/gorm"
	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
	"hazop-safeguard-coverage/backend/internal/repository"
	"hazop-safeguard-coverage/backend/internal/util"
)

type SafeguardOutageService interface {
	Register(context.Context, uint, dto.RegisterSafeguardOutageRequest, util.Actor) (dto.SafeguardOutageResponse, error)
	List(context.Context, dto.SafeguardOutageQuery) (dto.SafeguardOutageListResponse, error)
	Revoke(context.Context, uint, dto.RevokeSafeguardOutageRequest, util.Actor) (dto.SafeguardOutageResponse, error)
}

type safeguardOutageService struct {
	safeguards repository.SafeguardRepository
	outages    repository.SafeguardOutageRepository
	audits     repository.AuditRepository
	now        func() time.Time
}

func NewSafeguardOutageService(
	safeguards repository.SafeguardRepository,
	outages repository.SafeguardOutageRepository,
	audits repository.AuditRepository,
) SafeguardOutageService {
	return &safeguardOutageService{
		safeguards: safeguards, outages: outages, audits: audits,
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
	if _, err := s.safeguards.GetByID(ctx, safeguardID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardOutageResponse{}, util.NotFound("safeguard")
		}
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard", err)
	}
	startsAt := request.StartsAt.UTC()
	endsAt := request.EndsAt.UTC()
	if !endsAt.After(startsAt) {
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "ends_at must be after starts_at")
	}
	now := s.now()
	outage := model.SafeguardOutage{
		SafeguardID: safeguardID, Reason: request.Reason,
		StartsAt: startsAt, EndsAt: endsAt,
		RegisteredBy: actor.UserID, RegisteredByName: actor.Username,
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.outages.CreateIfNoOverlap(ctx, &outage)
	if err != nil {
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to register safeguard outage", err)
	}
	if !created {
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusConflict, util.CodeConflict, "outage window overlaps an already registered window for this safeguard")
	}
	if err := s.recordAudit(ctx, actor, outage.ID, "register", nil, outage, request.Reason); err != nil {
		return dto.SafeguardOutageResponse{}, err
	}
	stored, err := s.outages.GetByID(ctx, outage.ID)
	if err != nil {
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to reload safeguard outage", err)
	}
	return dto.NewSafeguardOutageResponse(stored, s.now()), nil
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
	before, err := s.outages.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardOutageResponse{}, util.NotFound("safeguard outage")
		}
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard outage", err)
	}
	now := s.now()
	switch before.StatusAt(now) {
	case model.OutageRevoked:
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusConflict, util.CodeStateTransition, "outage registration is already revoked")
	case model.OutageEnded:
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusConflict, util.CodeStateTransition, "an ended outage window cannot be revoked")
	}
	changed, err := s.outages.Revoke(ctx, id, map[string]any{
		"revoked_by": actor.UserID, "revoked_by_name": actor.Username,
		"revoked_at": now, "revoke_reason": request.Reason,
	}, now)
	if err != nil {
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to revoke safeguard outage", err)
	}
	if !changed {
		return dto.SafeguardOutageResponse{}, util.NewError(http.StatusConflict, util.CodeConflict, "outage registration changed concurrently")
	}
	after, err := s.outages.GetByID(ctx, id)
	if err != nil {
		return dto.SafeguardOutageResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to reload safeguard outage", err)
	}
	if err := s.recordAudit(ctx, actor, id, "revoke", before, after, request.Reason); err != nil {
		return dto.SafeguardOutageResponse{}, err
	}
	return dto.NewSafeguardOutageResponse(after, s.now()), nil
}

func (s *safeguardOutageService) recordAudit(
	ctx context.Context,
	actor util.Actor,
	entityID uint,
	action string,
	before any,
	after any,
	summary string,
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
		ActorRole: actor.Role, EntityType: "safeguard_outage", EntityID: entityID, Action: action,
		BeforeSnapshot: beforeJSON, AfterSnapshot: afterJSON,
		ResultSummary: util.CompactText(summary, 1000), CreatedAt: s.now(),
	}
	if err := s.audits.Record(ctx, log); err != nil {
		return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to record write audit", err)
	}
	return nil
}
