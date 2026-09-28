package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/innox-la/innox-zkteco-receiver/internal/service"
	"github.com/innox-la/innox-zkteco-receiver/web"
)

type DashboardHandler struct {
	svc *service.Service
}

func NewDashboardHandler(svc *service.Service) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

func (h *DashboardHandler) GetEventsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := h.svc.Subscribe()
	defer h.svc.Unsubscribe(ch)

	// Send initial comment to establish stream
	_, _ = fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err == nil {
				_, _ = fmt.Fprintf(w, "event: punch\ndata: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}

func (h *DashboardHandler) ListPunches(w http.ResponseWriter, r *http.Request) {
	events := h.svc.Repo().ListRecentEvents(100)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

func (h *DashboardHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	devices := h.svc.Repo().ListDevices()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(devices)
}

func (h *DashboardHandler) TestPunch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PIN        string `json:"pin"`
		VerifyType int    `json:"verify_type"`
		SN         string `json:"sn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if req.PIN == "" {
		req.PIN = "13" // Default to Billion
	}
	if req.SN == "" {
		req.SN = "PYA8262400422"
	}
	if req.VerifyType == 0 {
		req.VerifyType = 15 // Face
	}

	punchTime := time.Now().Format("2006-01-02 15:04:05")
	payload := fmt.Sprintf("%s\t%s\t0\t%d\t0\t0\t0\n", req.PIN, punchTime, req.VerifyType)

	count, err := h.svc.ProcessPunchPayload(r.Context(), req.SN, "127.0.0.1", []byte(payload))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   count,
		"pin":     req.PIN,
		"time":    punchTime,
	})
}

func (h *DashboardHandler) RenderDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(web.IndexHTML)
}
