package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/innox-la/innox-zkteco-receiver/internal/config"
	"github.com/innox-la/innox-zkteco-receiver/internal/model"
	"github.com/innox-la/innox-zkteco-receiver/internal/repository"
)

type Service struct {
	cfg  *config.Config
	repo *repository.Repository

	// SSE subscriber hub
	hubMu       sync.RWMutex
	subscribers map[chan *model.AttendanceEvent]struct{}
}

func New(cfg *config.Config, repo *repository.Repository) *Service {
	return &Service{
		cfg:         cfg,
		repo:        repo,
		subscribers: make(map[chan *model.AttendanceEvent]struct{}),
	}
}

func (s *Service) ProcessHandshake(ctx context.Context, sn, clientIP, pushVer, lang string) string {
	dev := &model.Device{
		SN:         sn,
		ClientIP:   clientIP,
		PushVer:    pushVer,
		Language:   lang,
		LastSeenAt: time.Now(),
		Status:     "ONLINE",
	}
	s.repo.UpsertDevice(ctx, dev)
	log.Printf("[adms] handshake from device SN=%s IP=%s PushVer=%s Lang=%s", sn, clientIP, pushVer, lang)

	return fmt.Sprintf("GET OPTION FROM: %s\n"+
		"RegistryCode=12345678901234567890\n"+
		"ServerVersion=3.1.2\n"+
		"ServerName=InnoxADMS\n"+
		"PushProtVer=3.1.2\n"+
		"ATTLOGStamp=None\n"+
		"OPERLOGStamp=None\n"+
		"ATTPHOTOStamp=None\n"+
		"ErrorDelay=60\n"+
		"Delay=10\n"+
		"TransTimes=00:00;23:59\n"+
		"TransInterval=1\n"+
		"TransFlag=TransData AttLog\tOpLog\tAttPhoto\n"+
		"Realtime=1\n"+
		"Encrypt=0\n", sn)
}

func (s *Service) ProcessRegistry(ctx context.Context, sn, clientIP, body string) string {
	dev, ok := s.repo.GetDevice(sn)
	if !ok {
		dev = &model.Device{
			SN:         sn,
			ClientIP:   clientIP,
			LastSeenAt: time.Now(),
			Status:     "ONLINE",
		}
	} else {
		dev.LastSeenAt = time.Now()
		dev.ClientIP = clientIP
		dev.Status = "ONLINE"
	}
	s.repo.UpsertDevice(ctx, dev)
	log.Printf("[adms] registry successful: SN=%s IP=%s payload=%q", sn, clientIP, body)

	return "registry=ok\n" +
		"RegistryCode=12345678901234567890\n" +
		"ServerVersion=3.1.2\n" +
		"ServerName=InnoxADMS\n" +
		"PushProtVer=3.1.2\n" +
		"ErrorDelay=60\n" +
		"RequestDelay=5\n" +
		"TransInterval=1\n" +
		"TransTimes=00:00;23:59\n" +
		"Realtime=1\n" +
		"OK\n"
}

func (s *Service) ProcessHeartbeat(ctx context.Context, sn, clientIP string) string {
	dev, ok := s.repo.GetDevice(sn)
	if !ok {
		dev = &model.Device{
			SN:         sn,
			ClientIP:   clientIP,
			LastSeenAt: time.Now(),
			Status:     "ONLINE",
		}
	} else {
		dev.LastSeenAt = time.Now()
		dev.ClientIP = clientIP
		dev.Status = "ONLINE"
	}
	s.repo.UpsertDevice(ctx, dev)
	return "OK\n"
}

func parsePunchLine(line string, loc *time.Location) (pin string, punchTime time.Time, punchState int, verifyType int, workCode string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", time.Time{}, 0, 0, "", false
	}

	// 1. Check if line has Key=Value format (e.g., PIN=1001 DateTime=2026-09-28 16:30:00 or tab separated)
	if strings.Contains(strings.ToUpper(line), "PIN=") {
		kv := make(map[string]string)
		var items []string
		if strings.Contains(line, "\t") {
			items = strings.Split(line, "\t")
		} else if strings.Contains(line, ",") {
			items = strings.Split(line, ",")
		} else {
			fields := strings.Fields(line)
			for i := 0; i < len(fields); i++ {
				f := fields[i]
				if strings.Contains(f, "=") {
					k := strings.ToLower(strings.SplitN(f, "=", 2)[0])
					v := strings.SplitN(f, "=", 2)[1]
					if (k == "datetime" || k == "time") && i+1 < len(fields) && !strings.Contains(fields[i+1], "=") {
						v = v + " " + fields[i+1]
						i++
					}
					kv[k] = v
				}
			}
		}

		for _, item := range items {
			parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
			if len(parts) == 2 {
				kv[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
			}
		}

		pin = kv["pin"]
		timeStr := kv["datetime"]
		if timeStr == "" {
			timeStr = kv["time"]
		}

		punchTime, _ = time.ParseInLocation("2006-01-02 15:04:05", timeStr, loc)
		if punchTime.IsZero() {
			punchTime = time.Now().In(loc)
		}

		punchState = 0
		if val, exists := kv["status"]; exists {
			fmt.Sscanf(val, "%d", &punchState)
		}

		verifyType = 15
		if val, exists := kv["verified"]; exists {
			fmt.Sscanf(val, "%d", &verifyType)
		}

		workCode = kv["workcode"]
		if pin != "" {
			return pin, punchTime, punchState, verifyType, workCode, true
		}
	}

	// 2. Positional format:
	// <PIN>\t<YYYY-MM-DD HH:MM:SS>\t<PunchState>\t<VerifyType>\t<WorkCode>
	parts := strings.Split(line, "\t")
	if len(parts) < 2 {
		parts = strings.Split(line, ",")
	}
	if len(parts) < 2 {
		return "", time.Time{}, 0, 0, "", false
	}

	pin = strings.TrimSpace(parts[0])
	timeStr := strings.TrimSpace(parts[1])

	punchTime, err := time.ParseInLocation("2006-01-02 15:04:05", timeStr, loc)
	if err != nil {
		punchTime = time.Now().In(loc)
	}

	punchState = 0
	if len(parts) >= 3 {
		fmt.Sscanf(parts[2], "%d", &punchState)
	}

	verifyType = 15
	if len(parts) >= 4 {
		fmt.Sscanf(parts[3], "%d", &verifyType)
	}

	if len(parts) >= 5 {
		workCode = parts[4]
	}

	return pin, punchTime, punchState, verifyType, workCode, true
}

func (s *Service) ProcessPunchPayload(ctx context.Context, sn, clientIP string, body []byte) (int, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	processedCount := 0

	loc, err := time.LoadLocation(s.cfg.Timezone)
	if err != nil {
		loc = time.FixedZone("ICT", 7*3600) // Vientiane / Bangkok UTC+7
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		pin, punchTime, punchState, verifyType, workCode, ok := parsePunchLine(line, loc)
		if !ok {
			log.Printf("[adms] unrecognized punch format: %s", line)
			continue
		}

		rawPunch := &model.RawPunch{
			SN:         sn,
			PIN:        pin,
			PunchTime:  punchTime,
			PunchState: punchState,
			VerifyType: verifyType,
			WorkCode:   workCode,
			RawLine:    line,
			CreatedAt:  time.Now(),
		}
		s.repo.SaveRawPunch(ctx, rawPunch)

		// Resolve employee name
		empName := fmt.Sprintf("PIN-%s", pin)
		if emp, found := s.repo.GetEmployee(pin); found {
			empName = emp.Name
		}

		verifyName := "Face Scan"
		switch verifyType {
		case 1:
			verifyName = "Fingerprint"
		case 2:
			verifyName = "Password"
		case 4:
			verifyName = "RFID Card"
		case 15:
			verifyName = "Face Scan"
		}

		dateStr := punchTime.Format("2006-01-02")
		firstCheckin := s.repo.GetFirstCheckinForPIN(pin, dateStr)
		lastEvent := s.repo.GetLastEventForPIN(pin, dateStr)

		event := &model.AttendanceEvent{
			ID:             fmt.Sprintf("EV-%d-%s", time.Now().UnixNano(), pin),
			SN:             sn,
			PIN:            pin,
			EmployeeName:   empName,
			EventTime:      punchTime,
			VerifyTypeName: verifyName,
		}

		if firstCheckin == nil {
			// First punch of the day: Check-in
			event.Action = "CHECK_IN"
			// 09:00 AM cutoff for ON_TIME / LATE
			cutoff := time.Date(punchTime.Year(), punchTime.Month(), punchTime.Day(), 9, 0, 0, 0, loc)
			if punchTime.Before(cutoff) || punchTime.Equal(cutoff) {
				event.Status = "ON_TIME"
			} else {
				event.Status = "LATE"
			}
		} else {
			// Already checked in today
			diff := punchTime.Sub(lastEvent.EventTime)
			debounceWindow := time.Duration(s.cfg.DebounceMinutes) * time.Minute

			if diff < debounceWindow {
				// Within debounce window: ignore duplicate
				event.Action = "DUPLICATE_IGNORED"
				event.Status = lastEvent.Status
			} else {
				// Check-out
				event.Action = "CHECK_OUT"
				event.Status = "COMPLETED"
				duration := punchTime.Sub(firstCheckin.EventTime)
				event.WorkDurationMinutes = int(duration.Minutes())
			}
		}

		s.repo.SaveEvent(ctx, event)
		s.Broadcast(event)
		processedCount++

		log.Printf("[adms] PUNCH PROCESSED: Employee=%s (PIN=%s) Action=%s Status=%s Time=%s Device=%s",
			event.EmployeeName, event.PIN, event.Action, event.Status, event.EventTime.Format("15:04:05"), sn)
	}

	return processedCount, nil
}

func (s *Service) Subscribe() chan *model.AttendanceEvent {
	s.hubMu.Lock()
	defer s.hubMu.Unlock()
	ch := make(chan *model.AttendanceEvent, 100)
	s.subscribers[ch] = struct{}{}
	return ch
}

func (s *Service) Unsubscribe(ch chan *model.AttendanceEvent) {
	s.hubMu.Lock()
	defer s.hubMu.Unlock()
	delete(s.subscribers, ch)
	close(ch)
}

func (s *Service) Broadcast(ev *model.AttendanceEvent) {
	s.hubMu.RLock()
	defer s.hubMu.RUnlock()
	for ch := range s.subscribers {
		select {
		case ch <- ev:
		default:
			// Buffer full, skip
		}
	}
}

func (s *Service) Repo() *repository.Repository {
	return s.repo
}
