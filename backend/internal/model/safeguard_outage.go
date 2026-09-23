package model

import "time"

// SafeguardOutage is a planned-maintenance outage window for one safeguard.
//
// A record is immutable once registered except for revocation: the registered
// window itself never changes, and overlapping windows for the same safeguard
// are rejected at registration. Revocation ends the window early and keeps the
// registration on file for the ledger and audit trail.
type SafeguardOutage struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	SafeguardID      uint       `gorm:"not null;index:idx_outage_safeguard_window,priority:1" json:"safeguard_id"`
	Safeguard        Safeguard  `gorm:"foreignKey:SafeguardID" json:"safeguard,omitempty"`
	Reason           string     `gorm:"type:text;not null" json:"reason"`
	StartsAt         time.Time  `gorm:"not null;index:idx_outage_safeguard_window,priority:2" json:"starts_at"`
	EndsAt           time.Time  `gorm:"not null;index:idx_outage_safeguard_window,priority:3" json:"ends_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevokedBy        *uint      `json:"revoked_by,omitempty"`
	RevokedByName    string     `gorm:"size:80" json:"revoked_by_name,omitempty"`
	RevokeReason     string     `gorm:"type:text" json:"revoke_reason,omitempty"`
	RegisteredBy     uint       `gorm:"not null" json:"registered_by"`
	RegisteredByName string     `gorm:"size:80;not null" json:"registered_by_name"`
	CreatedAt        time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"not null" json:"updated_at"`
}

func (SafeguardOutage) TableName() string { return "safeguard_outages" }

// Overlaps reports whether the half-open window [startsAt, endsAt) overlaps
// another window for the same safeguard. Touching endpoints are allowed: a new
// window may start exactly when an existing window ends.
func (o SafeguardOutage) Overlaps(startsAt, endsAt time.Time) bool {
	return o.StartsAt.Before(endsAt) && startsAt.Before(o.EndsAt)
}

// Covers reports whether the supplied instant falls inside the registered
// window. The end instant itself is considered back in service.
func (o SafeguardOutage) Covers(at time.Time) bool {
	return (at.After(o.StartsAt) || at.Equal(o.StartsAt)) && at.Before(o.EndsAt)
}
