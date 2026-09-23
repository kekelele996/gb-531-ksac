package dto

import (
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
}

type RevokeSafeguardOutageRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=1000"`
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
	SafeguardName    string     `json:"safeguard_name"`
	ScenarioID       uint       `json:"scenario_id"`
	Reason           string     `json:"reason"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           time.Time  `json:"ends_at"`
	Status           string     `json:"status"`
	RegisteredBy     uint       `json:"registered_by"`
	RegisteredByName string     `json:"registered_by_name"`
	RevokedBy        *uint      `json:"revoked_by,omitempty"`
	RevokedByName    string     `json:"revoked_by_name,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevokeReason     string     `json:"revoke_reason,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type SafeguardOutageListResponse struct {
	Items []SafeguardOutageResponse `json:"items"`
	Total int64                     `json:"total"`
	Page  int                       `json:"page"`
	Size  int                       `json:"page_size"`
}

func NewSafeguardOutageResponse(outage model.SafeguardOutage, now time.Time) SafeguardOutageResponse {
	return SafeguardOutageResponse{
		ID: outage.ID, SafeguardID: outage.SafeguardID,
		SafeguardName: outage.Safeguard.Name, ScenarioID: outage.Safeguard.TargetScenarioID,
		Reason: outage.Reason, StartsAt: outage.StartsAt, EndsAt: outage.EndsAt,
		Status: outage.StatusAt(now),
		RegisteredBy: outage.RegisteredBy, RegisteredByName: outage.RegisteredByName,
		RevokedBy: outage.RevokedBy, RevokedByName: outage.RevokedByName,
		RevokedAt: outage.RevokedAt, RevokeReason: outage.RevokeReason,
		CreatedAt: outage.CreatedAt,
	}
}
