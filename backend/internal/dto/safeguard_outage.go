package dto

import (
	"hazop-safeguard-coverage/backend/internal/constants"
	"hazop-safeguard-coverage/backend/internal/model"
	"strings"
	"time"
)

type RegisterSafeguardOutageRequest struct {
	Reason   string    `json:"reason" binding:"required,min=3,max=1000"`
	StartsAt time.Time `json:"starts_at" binding:"required"`
	EndsAt   time.Time `json:"ends_at" binding:"required"`
}

func (r *RegisterSafeguardOutageRequest) Normalize() {
	r.Reason = strings.TrimSpace(r.Reason)
	r.StartsAt = r.StartsAt.UTC().Truncate(time.Second)
	r.EndsAt = r.EndsAt.UTC().Truncate(time.Second)
}

type RevokeSafeguardOutageRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=1000"`
}

func (r *RevokeSafeguardOutageRequest) Normalize() {
	r.Reason = strings.TrimSpace(r.Reason)
}

type SafeguardOutageQuery struct {
	SafeguardID uint
	ScenarioID  uint
	Status      string
	Page        int
	PageSize    int
}

type SafeguardOutageResponse struct {
	ID               uint       `json:"id"`
	SafeguardID      uint       `json:"safeguard_id"`
	SafeguardName    string     `json:"safeguard_name,omitempty"`
	Reason           string     `json:"reason"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           time.Time  `json:"ends_at"`
	Status           string     `json:"status"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevokedBy        *uint      `json:"revoked_by,omitempty"`
	RevokedByName    string     `json:"revoked_by_name,omitempty"`
	RevokeReason     string     `json:"revoke_reason,omitempty"`
	RegisteredBy     uint       `json:"registered_by"`
	RegisteredByName string     `json:"registered_by_name"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type SafeguardOutageListResponse struct {
	Items []SafeguardOutageResponse `json:"items"`
	Total int64                     `json:"total"`
	Page  int                       `json:"page"`
	Size  int                       `json:"page_size"`
}

func NewSafeguardOutageResponse(o model.SafeguardOutage, now time.Time) SafeguardOutageResponse {
	status := string(constants.DeriveOutageState(o.RevokedAt != nil, o.StartsAt, o.EndsAt, now))
	name := ""
	if o.Safeguard.ID != 0 {
		name = o.Safeguard.Name
	}
	return SafeguardOutageResponse{
		ID: o.ID, SafeguardID: o.SafeguardID, SafeguardName: name,
		Reason: o.Reason, StartsAt: o.StartsAt, EndsAt: o.EndsAt, Status: status,
		RevokedAt: o.RevokedAt, RevokedBy: o.RevokedBy, RevokedByName: o.RevokedByName,
		RevokeReason: o.RevokeReason, RegisteredBy: o.RegisteredBy,
		RegisteredByName: o.RegisteredByName, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}
