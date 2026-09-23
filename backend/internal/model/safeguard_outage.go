package model

import "time"

const (
	OutageScheduled = "scheduled"
	OutageActive    = "active"
	OutageEnded     = "ended"
	OutageRevoked   = "revoked"
)

// SafeguardOutage is a registered planned maintenance window during which a
// safeguard is taken out of service. Eligibility is derived from the window
// and the revocation marker only, so a safeguard is automatically counted
// again once the window ends.
type SafeguardOutage struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	SafeguardID      uint       `gorm:"not null;index:idx_outage_safeguard_window" json:"safeguard_id"`
	Safeguard        Safeguard  `gorm:"foreignKey:SafeguardID" json:"safeguard,omitempty"`
	Reason           string     `gorm:"type:text;not null" json:"reason"`
	StartsAt         time.Time  `gorm:"not null;index:idx_outage_safeguard_window" json:"starts_at"`
	EndsAt           time.Time  `gorm:"not null" json:"ends_at"`
	RegisteredBy     uint       `gorm:"not null;index" json:"registered_by"`
	RegisteredByName string     `gorm:"size:80;not null" json:"registered_by_name"`
	RevokedBy        *uint      `json:"revoked_by,omitempty"`
	RevokedByName    string     `gorm:"size:80" json:"revoked_by_name,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevokeReason     string     `gorm:"type:text" json:"revoke_reason,omitempty"`
	CreatedAt        time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"not null" json:"updated_at"`
}

func (SafeguardOutage) TableName() string { return "safeguard_outages" }

// StatusAt derives the ledger status of the registration at the given moment.
func (o SafeguardOutage) StatusAt(now time.Time) string {
	if o.RevokedAt != nil {
		return OutageRevoked
	}
	if now.Before(o.StartsAt) {
		return OutageScheduled
	}
	if now.Before(o.EndsAt) {
		return OutageActive
	}
	return OutageEnded
}

// Covers reports whether the safeguard is out of service at the given moment,
// e.g. when an evaluation snapshot freeze time falls inside the window.
func (o SafeguardOutage) Covers(at time.Time) bool {
	return o.RevokedAt == nil && !at.Before(o.StartsAt) && at.Before(o.EndsAt)
}
