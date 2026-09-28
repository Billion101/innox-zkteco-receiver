package handler

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/innox-la/innox-zkteco-receiver/internal/service"
)

type ADMSHandler struct {
	svc *service.Service
}

func NewADMSHandler(svc *service.Service) *ADMSHandler {
	return &ADMSHandler{svc: svc}
}

// GetCData handles the handshake and configuration requests from ZKTeco
// Request: GET /iclock/cdata?SN=PYA8262400422&options=all&language=83&pushver=...
func (h *ADMSHandler) GetCData(w http.ResponseWriter, r *http.Request) {
	sn := r.URL.Query().Get("SN")
	if sn == "" {
		sn = "UNKNOWN"
	}

	pushVer := r.URL.Query().Get("pushver")
	lang := r.URL.Query().Get("language")
	clientIP := getClientIP(r)

	respText := h.svc.ProcessHandshake(r.Context(), sn, clientIP, pushVer, lang)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(respText))
}

// PostCData handles punch logs pushed by ZKTeco
// Request: POST /iclock/cdata?SN=PYA8262400422&table=ATTLOG&Stamp=...
func (h *ADMSHandler) PostCData(w http.ResponseWriter, r *http.Request) {
	sn := r.URL.Query().Get("SN")
	table := r.URL.Query().Get("table")
	clientIP := getClientIP(r)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[adms] failed to read body from SN=%s: %v", sn, err)
		http.Error(w, "ERROR", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("[adms] received POST table=%s from SN=%s (size=%d bytes, ip=%s)", table, sn, len(body), clientIP)

	count, err := h.svc.ProcessPunchPayload(r.Context(), sn, clientIP, body)
	if err != nil {
		log.Printf("[adms] error processing punches: %v", err)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if count > 0 {
		_, _ = fmt.Fprintf(w, "OK: %d\n", count)
	} else {
		_, _ = w.Write([]byte("OK\n"))
	}
}

// GetRequest handles device polling and heartbeat
// Request: GET /iclock/getrequest?SN=PYA8262400422
func (h *ADMSHandler) GetRequest(w http.ResponseWriter, r *http.Request) {
	sn := r.URL.Query().Get("SN")
	clientIP := getClientIP(r)

	resp := h.svc.ProcessHeartbeat(r.Context(), sn, clientIP)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(resp))
}

// DeviceCmd handles command execution acknowledgments from the device
// Request: POST /iclock/devicecmd?SN=PYA8262400422
func (h *ADMSHandler) DeviceCmd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK\n"))
}

// FileData handles any biometric photo / template uploads
func (h *ADMSHandler) FileData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK\n"))
}

func getClientIP(r *http.Request) string {
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		parts := strings.Split(xForwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}
	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		return strings.TrimSpace(xRealIP)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
