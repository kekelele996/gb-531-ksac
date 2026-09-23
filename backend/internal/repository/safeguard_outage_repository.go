package repository

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
	"time"
)

type SafeguardOutageRepository interface {
	Create(ctx context.Context, tx *gorm.DB, outage *model.SafeguardOutage) error
	GetByID(ctx context.Context, id uint) (model.SafeguardOutage, error)
	List(ctx context.Context, query dto.SafeguardOutageQuery, now time.Time) ([]model.SafeguardOutage, int64, error)
	// ListEffectiveBySafeguard returns every non-revoked registration for a
	// safeguard, ordered by window start. It validates overlap at registration
	// and must run on the caller's transaction connection (SQLite uses a single
	// connection, so a second connection inside the transaction would deadlock).
	ListEffectiveBySafeguard(ctx context.Context, conn *gorm.DB, safeguardID uint) ([]model.SafeguardOutage, error)
	// ActiveAt returns non-revoked outage windows that cover the supplied instant
	// for the given safeguards. Used to freeze out-of-service safeguards into a
	// coverage snapshot.
	ActiveAt(ctx context.Context, safeguardIDs []uint, at time.Time) ([]model.SafeguardOutage, error)
	Revoke(ctx context.Context, id uint, updates map[string]any) (bool, error)
}

type safeguardOutageRepository struct{ db *gorm.DB }

func NewSafeguardOutageRepository(db *gorm.DB) SafeguardOutageRepository {
	return &safeguardOutageRepository{db: db}
}

func (r *safeguardOutageRepository) Create(ctx context.Context, tx *gorm.DB, outage *model.SafeguardOutage) error {
	conn := r.db
	if tx != nil {
		conn = tx
	}
	if err := conn.WithContext(ctx).Create(outage).Error; err != nil {
		return fmt.Errorf("create safeguard outage: %w", err)
	}
	return nil
}

func (r *safeguardOutageRepository) GetByID(ctx context.Context, id uint) (model.SafeguardOutage, error) {
	var outage model.SafeguardOutage
	if err := r.db.WithContext(ctx).Preload("Safeguard").First(&outage, id).Error; err != nil {
		return model.SafeguardOutage{}, fmt.Errorf("find safeguard outage %d: %w", id, err)
	}
	return outage, nil
}

func (r *safeguardOutageRepository) List(ctx context.Context, query dto.SafeguardOutageQuery, now time.Time) ([]model.SafeguardOutage, int64, error) {
	base := r.db.WithContext(ctx).Model(&model.SafeguardOutage{})
	if query.SafeguardID != 0 {
		base = base.Where("safeguard_id = ?", query.SafeguardID)
	}
	if query.ScenarioID != 0 {
		base = base.Joins("JOIN safeguards ON safeguards.id = safeguard_outages.safeguard_id").
			Where("safeguards.target_scenario_id = ?", query.ScenarioID)
	}
	switch query.Status {
	case "active":
		base = base.Where("revoked_at IS NULL AND starts_at <= ? AND ends_at > ?", now, now)
	case "pending":
		base = base.Where("revoked_at IS NULL AND starts_at > ?", now)
	case "ended":
		base = base.Where("revoked_at IS NOT NULL OR ends_at <= ?", now)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count safeguard outages: %w", err)
	}
	var outages []model.SafeguardOutage
	offset := (query.Page - 1) * query.PageSize
	if err := base.Preload("Safeguard").
		Order("starts_at DESC, id DESC").
		Limit(query.PageSize).Offset(offset).Find(&outages).Error; err != nil {
		return nil, 0, fmt.Errorf("list safeguard outages: %w", err)
	}
	return outages, total, nil
}

func (r *safeguardOutageRepository) ListEffectiveBySafeguard(ctx context.Context, conn *gorm.DB, safeguardID uint) ([]model.SafeguardOutage, error) {
	var outages []model.SafeguardOutage
	if err := conn.WithContext(ctx).
		Where("safeguard_id = ? AND revoked_at IS NULL", safeguardID).
		Order("starts_at ASC, id ASC").Find(&outages).Error; err != nil {
		return nil, fmt.Errorf("list effective outages for safeguard %d: %w", safeguardID, err)
	}
	return outages, nil
}

func (r *safeguardOutageRepository) ActiveAt(ctx context.Context, safeguardIDs []uint, at time.Time) ([]model.SafeguardOutage, error) {
	var outages []model.SafeguardOutage
	if len(safeguardIDs) == 0 {
		return outages, nil
	}
	if err := r.db.WithContext(ctx).
		Where("safeguard_id IN ? AND revoked_at IS NULL AND starts_at <= ? AND ends_at > ?", safeguardIDs, at, at).
		Order("starts_at ASC, id ASC").Find(&outages).Error; err != nil {
		return nil, fmt.Errorf("find active outages at %s: %w", at.Format(time.RFC3339), err)
	}
	return outages, nil
}

func (r *safeguardOutageRepository) Revoke(ctx context.Context, id uint, updates map[string]any) (bool, error) {
	values := make(map[string]any, len(updates)+1)
	for key, value := range updates {
		values[key] = value
	}
	values["updated_at"] = time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&model.SafeguardOutage{}).
		Where("id = ? AND revoked_at IS NULL", id).Updates(values)
	if result.Error != nil {
		return false, fmt.Errorf("revoke safeguard outage %d: %w", id, result.Error)
	}
	return result.RowsAffected == 1, nil
}
