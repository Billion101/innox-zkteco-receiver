package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/innox-la/innox-zkteco-receiver/internal/config"
	"github.com/innox-la/innox-zkteco-receiver/internal/repository"
	"github.com/innox-la/innox-zkteco-receiver/internal/service"
)

func TestADMSHandshakeAndHeartbeat(t *testing.T) {
	cfg := &config.Config{
		Port:            "8087",
		Timezone:        "Asia/Vientiane",
		DebounceMinutes: 5,
	}
	repo := repository.New(nil) // Memory mode
	svc := service.New(cfg, repo)

	sn := "PYA8262400422"
	ip := "192.168.1.201"

	// 1. Test Handshake
	resp := svc.ProcessHandshake(context.Background(), sn, ip, "2.4.1", "83")
	if !strings.Contains(resp, "GET OPTION FROM: "+sn) {
		t.Fatalf("expected response to contain GET OPTION FROM: %s, got: %s", sn, resp)
	}
	if !strings.Contains(resp, "Realtime=1") {
		t.Fatalf("expected Realtime=1 in response, got: %s", resp)
	}

	dev, ok := repo.GetDevice(sn)
	if !ok || dev.Status != "ONLINE" {
		t.Fatalf("expected device %s to be ONLINE, got: %v", sn, dev)
	}

	// 2. Test Heartbeat
	hbResp := svc.ProcessHeartbeat(context.Background(), sn, ip)
	if hbResp != "OK\n" {
		t.Fatalf("expected OK\\n, got %q", hbResp)
	}
}

func TestPunchProcessingAndSmartLogic(t *testing.T) {
	cfg := &config.Config{
		Port:            "8087",
		Timezone:        "Asia/Vientiane",
		DebounceMinutes: 5,
	}
	repo := repository.New(nil)
	svc := service.New(cfg, repo)

	sn := "PYA8262400422"
	ip := "192.168.1.201"

	// Register device first
	svc.ProcessHandshake(context.Background(), sn, ip, "2.4.1", "83")

	// Subscribe to events
	evCh := svc.Subscribe()
	defer svc.Unsubscribe(evCh)

	// Punch 1: Billion (PIN 13) at 08:45:00 (On time check-in)
	payload1 := "13\t2026-09-28 08:45:00\t0\t15\t0\t0\t0\n"
	count, err := svc.ProcessPunchPayload(context.Background(), sn, ip, []byte(payload1))
	if err != nil || count != 1 {
		t.Fatalf("expected 1 processed punch, got count=%d, err=%v", count, err)
	}

	select {
	case ev := <-evCh:
		if ev.EmployeeName != "billion" {
			t.Errorf("expected employee name 'billion', got %s", ev.EmployeeName)
		}
		if ev.Action != "CHECK_IN" {
			t.Errorf("expected action 'CHECK_IN', got %s", ev.Action)
		}
		if ev.Status != "ON_TIME" {
			t.Errorf("expected status 'ON_TIME', got %s", ev.Status)
		}
		if ev.VerifyTypeName != "Face Scan" {
			t.Errorf("expected verify type 'Face Scan', got %s", ev.VerifyTypeName)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event 1")
	}

	// Punch 2: Duplicate scan within 1 minute (Debounce test)
	payload2 := "13\t2026-09-28 08:46:00\t0\t15\t0\t0\t0\n"
	_, _ = svc.ProcessPunchPayload(context.Background(), sn, ip, []byte(payload2))
	select {
	case ev := <-evCh:
		if ev.Action != "DUPLICATE_IGNORED" {
			t.Errorf("expected action 'DUPLICATE_IGNORED', got %s", ev.Action)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for debounce event")
	}

	// Punch 3: Check-out at 17:30:00 (Evening punch)
	payload3 := "13\t2026-09-28 17:30:00\t0\t15\t0\t0\t0\n"
	_, _ = svc.ProcessPunchPayload(context.Background(), sn, ip, []byte(payload3))
	select {
	case ev := <-evCh:
		if ev.Action != "CHECK_OUT" {
			t.Errorf("expected action 'CHECK_OUT', got %s", ev.Action)
		}
		if ev.WorkDurationMinutes <= 0 {
			t.Errorf("expected work duration > 0 minutes, got %d", ev.WorkDurationMinutes)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for checkout event")
	}
}
