package repository

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/innox-la/innox-zkteco-receiver/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool

	// In-memory state for lightning-fast reads & standalone test mode
	mu           sync.RWMutex
	devices      map[string]*model.Device
	employees    map[string]*model.Employee
	rawPunches   []*model.RawPunch
	recentEvents []*model.AttendanceEvent
}

func New(pool *pgxpool.Pool) *Repository {
	repo := &Repository{
		pool:         pool,
		devices:      make(map[string]*model.Device),
		employees:    make(map[string]*model.Employee),
		rawPunches:   make([]*model.RawPunch, 0, 1000),
		recentEvents: make([]*model.AttendanceEvent, 0, 1000),
	}

	// Seed known users from ZKTeco terminal (PINs 9 to 16)
	defaultEmployees := []*model.Employee{
		{PIN: "9", Name: "TINAR", Department: "Engineering"},
		{PIN: "10", Name: "singthou", Department: "Engineering"},
		{PIN: "11", Name: "sengsourath", Department: "Operations"},
		{PIN: "12", Name: "ketsana", Department: "Product"},
		{PIN: "13", Name: "billion", Department: "Core Engineering"},
		{PIN: "14", Name: "anouluck", Department: "QA"},
		{PIN: "15", Name: "XANAKONE", Department: "Management"},
		{PIN: "16", Name: "saymisouk", Department: "Core Engineering"},
	}

	for _, emp := range defaultEmployees {
		repo.employees[emp.PIN] = emp
	}

	return repo
}

func (r *Repository) EnsureSchema(ctx context.Context) error {
	if r.pool == nil {
		log.Println("[repo] running in memory-only mode (no PostgreSQL pool connected)")
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS zkteco_devices (
		sn VARCHAR(100) PRIMARY KEY,
		client_ip VARCHAR(50),
		push_ver VARCHAR(50),
		language VARCHAR(20),
		last_seen_at TIMESTAMPTZ NOT NULL,
		status VARCHAR(20) DEFAULT 'ONLINE'
	);

	CREATE TABLE IF NOT EXISTS zkteco_users (
		pin VARCHAR(50) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		department VARCHAR(100) DEFAULT 'General',
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS zkteco_punches (
		id BIGSERIAL PRIMARY KEY,
		sn VARCHAR(100) NOT NULL,
		pin VARCHAR(50) NOT NULL,
		punch_time TIMESTAMPTZ NOT NULL,
		punch_state INT DEFAULT 0,
		verify_type INT DEFAULT 15,
		work_code VARCHAR(50),
		raw_line TEXT,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS zkteco_events (
		id VARCHAR(100) PRIMARY KEY,
		sn VARCHAR(100) NOT NULL,
		pin VARCHAR(50) NOT NULL,
		employee_name VARCHAR(100) NOT NULL,
		action VARCHAR(30) NOT NULL,
		status VARCHAR(30) NOT NULL,
		event_time TIMESTAMPTZ NOT NULL,
		work_duration_minutes INT DEFAULT 0,
		verify_type_name VARCHAR(50),
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
	`

	_, err := r.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Seed default employees in DB
	for _, emp := range r.employees {
		_, _ = r.pool.Exec(ctx, `
			INSERT INTO zkteco_users (pin, name, department)
			VALUES ($1, $2, $3)
			ON CONFLICT (pin) DO UPDATE SET name = EXCLUDED.name, department = EXCLUDED.department;
		`, emp.PIN, emp.Name, emp.Department)
	}

	log.Println("[repo] PostgreSQL schema and seed data verified successfully")
	return nil
}

func (r *Repository) UpsertDevice(ctx context.Context, dev *model.Device) {
	r.mu.Lock()
	r.devices[dev.SN] = dev
	r.mu.Unlock()

	if r.pool != nil {
		go func() {
			_, err := r.pool.Exec(context.Background(), `
				INSERT INTO zkteco_devices (sn, client_ip, push_ver, language, last_seen_at, status)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (sn) DO UPDATE 
				SET client_ip = EXCLUDED.client_ip,
				    push_ver = EXCLUDED.push_ver,
				    language = EXCLUDED.language,
				    last_seen_at = EXCLUDED.last_seen_at,
				    status = EXCLUDED.status;
			`, dev.SN, dev.ClientIP, dev.PushVer, dev.Language, dev.LastSeenAt, dev.Status)
			if err != nil {
				log.Printf("[repo] error upserting device: %v", err)
			}
		}()
	}
}

func (r *Repository) GetDevice(sn string) (*model.Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	dev, ok := r.devices[sn]
	return dev, ok
}

func (r *Repository) ListDevices() []*model.Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*model.Device, 0, len(r.devices))
	for _, d := range r.devices {
		copyDev := *d
		// Devices send heartbeats every 30-60s. If older than 90s, mark as OFFLINE
		if time.Since(copyDev.LastSeenAt) > 90*time.Second {
			copyDev.Status = "OFFLINE"
		} else {
			copyDev.Status = "ONLINE"
		}
		list = append(list, &copyDev)
	}
	return list
}

func (r *Repository) GetEmployee(pin string) (*model.Employee, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	emp, ok := r.employees[pin]
	return emp, ok
}

func (r *Repository) SaveRawPunch(ctx context.Context, punch *model.RawPunch) {
	r.mu.Lock()
	r.rawPunches = append([]*model.RawPunch{punch}, r.rawPunches...)
	if len(r.rawPunches) > 500 {
		r.rawPunches = r.rawPunches[:500]
	}
	r.mu.Unlock()

	if r.pool != nil {
		go func() {
			_, err := r.pool.Exec(context.Background(), `
				INSERT INTO zkteco_punches (sn, pin, punch_time, punch_state, verify_type, work_code, raw_line, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
			`, punch.SN, punch.PIN, punch.PunchTime, punch.PunchState, punch.VerifyType, punch.WorkCode, punch.RawLine, punch.CreatedAt)
			if err != nil {
				log.Printf("[repo] error saving punch: %v", err)
			}
		}()
	}
}

func (r *Repository) SaveEvent(ctx context.Context, event *model.AttendanceEvent) {
	r.mu.Lock()
	r.recentEvents = append([]*model.AttendanceEvent{event}, r.recentEvents...)
	if len(r.recentEvents) > 500 {
		r.recentEvents = r.recentEvents[:500]
	}
	r.mu.Unlock()

	if r.pool != nil {
		go func() {
			_, err := r.pool.Exec(context.Background(), `
				INSERT INTO zkteco_events (id, sn, pin, employee_name, action, status, event_time, work_duration_minutes, verify_type_name)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
			`, event.ID, event.SN, event.PIN, event.EmployeeName, event.Action, event.Status, event.EventTime, event.WorkDurationMinutes, event.VerifyTypeName)
			if err != nil {
				log.Printf("[repo] error saving event: %v", err)
			}
		}()
	}
}

func (r *Repository) GetLastEventForPIN(pin string, date string) *model.AttendanceEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, ev := range r.recentEvents {
		if ev.PIN == pin && ev.EventTime.Format("2006-01-02") == date {
			return ev
		}
	}
	return nil
}

func (r *Repository) GetFirstCheckinForPIN(pin string, date string) *model.AttendanceEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var first *model.AttendanceEvent
	for i := len(r.recentEvents) - 1; i >= 0; i-- {
		ev := r.recentEvents[i]
		if ev.PIN == pin && ev.EventTime.Format("2006-01-02") == date && ev.Action == "CHECK_IN" {
			first = ev
			break
		}
	}
	return first
}

func (r *Repository) ListRecentEvents(limit int) []*model.AttendanceEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 || limit > len(r.recentEvents) {
		limit = len(r.recentEvents)
	}
	res := make([]*model.AttendanceEvent, limit)
	copy(res, r.recentEvents[:limit])
	return res
}

func (r *Repository) ListEmployees() []*model.Employee {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*model.Employee, 0, len(r.employees))
	for _, e := range r.employees {
		list = append(list, e)
	}
	return list
}
