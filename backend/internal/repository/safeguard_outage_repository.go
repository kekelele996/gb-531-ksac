package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
)

type SafeguardOutageRepository interface {
	CreateIfNoOverlap(context.Context, *model.SafeguardOutage) (bool, error)
	GetByID(context.Context, uint) (model.SafeguardOutage, error)
	List(context.Context, dto.SafeguardOutageQuery, time.Time) ([]model.SafeguardOutage, int64, error)
	ActiveCovering(context.Context, []uint, time.Time) ([]model.SafeguardOutage, error)
	Revoke(context.Context, uint, map[string]any, time.Time) (bool, error)
}

type safeguardOutageRepository struct{ db *gorm.DB }

func NewSafeguardOutageRepository(db *gorm.DB) SafeguardOutageRepository {
	return &safeguardOutageRepository{db: db}
}

// CreateIfNoOverlap inserts the registration only when no other effective
// (non-revoked) window for the same safeguard overlaps it. The parent
// safeguard row is locked first so concurrent submissions are serialized:
// a later, overlapping registration never takes effect.
func (r *safeguardOutageRepository) CreateIfNoOverlap(ctx context.Context, outage *model.SafeguardOutage) (bool, error) {
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").First(&model.Safeguard{}, outage.SafeguardID).Error; err != nil {
			return fmt.Errorf("lock safeguard %d: %w", outage.SafeguardID, err)
		}
		var conflicts int64
		if err := tx.Model(&model.SafeguardOutage{}).
			Where("safeguard_id = ? AND revoked_at IS NULL", outage.SafeguardID).
			Where("starts_at < ? AND ends_at > ?", outage.EndsAt, outage.StartsAt).
			Count(&conflicts).Error; err != nil {
			return fmt.Errorf("check safeguard outage overlap: %w", err)
		}
		if conflicts > 0 {
			return nil
		}
		if err := tx.Create(outage).Error; err != nil {
			return fmt.Errorf("create safeguard outage: %w", err)
		}
		created = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, nil
}

func (r *safeguardOutageRepository) GetByID(ctx context.Context, id uint) (model.SafeguardOutage, error) {
	var outage model.SafeguardOutage
	if err := r.db.WithContext(ctx).Preload("Safeguard").First(&outage, id).Error; err != nil {
		return model.SafeguardOutage{}, fmt.Errorf("find safeguard outage %d: %w", id, err)
	}
	return outage, nil
}

func (r *safeguardOutageRepository) List(
	ctx context.Context,
	query dto.SafeguardOutageQuery,
	now time.Time,
) ([]model.SafeguardOutage, int64, error) {
	base := r.db.WithContext(ctx).Model(&model.SafeguardOutage{}).
		Joins("JOIN safeguards ON safeguards.id = safeguard_outages.safeguard_id")
	if query.SafeguardID != 0 {
		base = base.Where("safeguard_outages.safeguard_id = ?", query.SafeguardID)
	}
	if query.ScenarioID != 0 {
		base = base.Where("safeguards.target_scenario_id = ?", query.ScenarioID)
	}
	switch query.Status {
	case model.OutageScheduled:
		base = base.Where("safeguard_outages.revoked_at IS NULL AND safeguard_outages.starts_at > ?", now)
	case model.OutageActive:
		base = base.Where("safeguard_outages.revoked_at IS NULL AND safeguard_outages.starts_at <= ? AND safeguard_outages.ends_at > ?", now, now)
	case model.OutageEnded:
		base = base.Where("safeguard_outages.revoked_at IS NULL AND safeguard_outages.ends_at <= ?", now)
	case model.OutageRevoked:
		base = base.Where("safeguard_outages.revoked_at IS NOT NULL")
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count safeguard outages: %w", err)
	}
	var outages []model.SafeguardOutage
	offset := (query.Page - 1) * query.PageSize
	if err := base.Preload("Safeguard").Order("safeguard_outages.starts_at DESC, safeguard_outages.id DESC").
		Limit(query.PageSize).Offset(offset).Find(&outages).Error; err != nil {
		return nil, 0, fmt.Errorf("list safeguard outages: %w", err)
	}
	return outages, total, nil
}

// ActiveCovering returns the effective registrations whose window contains the
// given moment, used to mark evaluation snapshots frozen at that moment.
func (r *safeguardOutageRepository) ActiveCovering(
	ctx context.Context,
	safeguardIDs []uint,
	at time.Time,
) ([]model.SafeguardOutage, error) {
	if len(safeguardIDs) == 0 {
		return nil, nil
	}
	var outages []model.SafeguardOutage
	if err := r.db.WithContext(ctx).
		Where("safeguard_id IN ? AND revoked_at IS NULL", safeguardIDs).
		Where("starts_at <= ? AND ends_at > ?", at, at).
		Order("safeguard_id ASC, id ASC").Find(&outages).Error; err != nil {
		return nil, fmt.Errorf("list active safeguard outages: %w", err)
	}
	return outages, nil
}

// Revoke marks a registration revoked. Only scheduled or in-progress windows
// can be revoked; ended and already revoked rows are left untouched.
func (r *safeguardOutageRepository) Revoke(
	ctx context.Context,
	id uint,
	updates map[string]any,
	now time.Time,
) (bool, error) {
	values := make(map[string]any, len(updates)+1)
	for key, value := range updates {
		values[key] = value
	}
	values["updated_at"] = now
	result := r.db.WithContext(ctx).Model(&model.SafeguardOutage{}).
		Where("id = ? AND revoked_at IS NULL AND ends_at > ?", id, now).
		Updates(values)
	if result.Error != nil {
		return false, fmt.Errorf("revoke safeguard outage %d: %w", id, result.Error)
	}
	return result.RowsAffected == 1, nil
}
