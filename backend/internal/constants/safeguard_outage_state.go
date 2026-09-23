package constants

import "time"

type SafeguardOutageState string

const (
	OutagePending SafeguardOutageState = "pending" // 待停用：已登记，停用窗口尚未开始
	OutageActive  SafeguardOutageState = "active"  // 停用中：当前时刻落在停用窗口内
	OutageEnded   SafeguardOutageState = "ended"   // 已结束：窗口自然到期，或被安全复核员提前撤销
)

func (s SafeguardOutageState) Valid() bool {
	switch s {
	case OutagePending, OutageActive, OutageEnded:
		return true
	}
	return false
}

func SafeguardOutageStateValues() []string {
	return []string{string(OutagePending), string(OutageActive), string(OutageEnded)}
}

// DeriveOutageState computes the ledger state of a registered outage window at
// the supplied observation time. Revoked windows stay ended regardless of time.
func DeriveOutageState(revoked bool, startsAt, endsAt, now time.Time) SafeguardOutageState {
	if revoked {
		return OutageEnded
	}
	if now.Before(startsAt) {
		return OutagePending
	}
	if now.After(endsAt) || now.Equal(endsAt) {
		return OutageEnded
	}
	return OutageActive
}
