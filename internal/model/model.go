package model

import "time"

type Device struct {
	SN         string    `json:"sn"`
	ClientIP   string    `json:"client_ip"`
	PushVer    string    `json:"push_ver"`
	Language   string    `json:"language"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Status     string    `json:"status"` // "ONLINE", "IDLE"
}

type Employee struct {
	PIN        string `json:"pin"`
	Name       string `json:"name"`
	Department string `json:"department"`
}

type RawPunch struct {
	ID         string    `json:"id"`
	SN         string    `json:"sn"`
	PIN        string    `json:"pin"`
	PunchTime  time.Time `json:"punch_time"`
	PunchState int       `json:"punch_state"` // 0: Check-In, 1: Check-Out, 4: Overtime-In, 5: Overtime-Out
	VerifyType int       `json:"verify_type"` // 1: Fingerprint, 15: Face, 2: Password, 4: Card
	WorkCode   string    `json:"work_code"`
	RawLine    string    `json:"raw_line"`
	CreatedAt  time.Time `json:"created_at"`
}

type AttendanceEvent struct {
	ID                  string    `json:"id"`
	SN                  string    `json:"sn"`
	PIN                 string    `json:"pin"`
	EmployeeName        string    `json:"employee_name"`
	Action              string    `json:"action"` // "CHECK_IN", "CHECK_OUT", "DUPLICATE_IGNORED"
	Status              string    `json:"status"` // "ON_TIME", "LATE"
	EventTime           time.Time `json:"event_time"`
	WorkDurationMinutes int       `json:"work_duration_minutes,omitempty"`
	VerifyTypeName      string    `json:"verify_type_name"`
}
