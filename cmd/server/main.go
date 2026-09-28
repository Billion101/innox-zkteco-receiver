package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/innox-la/innox-zkteco-receiver/internal/config"
	"github.com/innox-la/innox-zkteco-receiver/internal/handler"
	"github.com/innox-la/innox-zkteco-receiver/internal/repository"
	"github.com/innox-la/innox-zkteco-receiver/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()

	var pool *pgxpool.Pool
	if cfg.DBDSN != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var err error
		pool, err = pgxpool.New(ctx, cfg.DBDSN)
		cancel()
		if err != nil {
			log.Printf("[main] warning: could not connect to PostgreSQL: %v (continuing with in-memory mode)", err)
		} else {
			log.Printf("[main] connected to PostgreSQL database")
		}
	} else {
		log.Printf("[main] DB_DSN not specified, running in standalone memory mode")
	}

	repo := repository.New(pool)
	if pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := repo.EnsureSchema(ctx); err != nil {
			log.Printf("[main] schema migration error: %v", err)
		}
		cancel()
	}

	svc := service.New(cfg, repo)
	admsHandler := handler.NewADMSHandler(svc)
	dashHandler := handler.NewDashboardHandler(svc)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health Check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// ZKTeco ADMS Device Endpoints (Outbound HTTP push from terminal)
	r.Get("/iclock/cdata", admsHandler.GetCData)
	r.Post("/iclock/cdata", admsHandler.PostCData)
	r.Get("/iclock/getrequest", admsHandler.GetRequest)
	r.Post("/iclock/devicecmd", admsHandler.DeviceCmd)
	r.Get("/iclock/fdata", admsHandler.FileData)
	r.Post("/iclock/fdata", admsHandler.FileData)

	// Web UI & Monitoring APIs
	r.Get("/", dashHandler.RenderDashboard)
	r.Get("/api/devices", dashHandler.ListDevices)
	r.Get("/api/punches", dashHandler.ListPunches)
	r.Get("/api/events/stream", dashHandler.GetEventsStream)
	r.Post("/api/test-punch", dashHandler.TestPunch)

	log.Printf("=====================================================")
	log.Printf(" INNO X ZKTeco ADMS Receiver starting on :%s", cfg.Port)
	log.Printf(" Device Handshake: http://0.0.0.0:%s/iclock/cdata", cfg.Port)
	log.Printf(" Dashboard UI:     http://0.0.0.0:%s/", cfg.Port)
	log.Printf("=====================================================")

	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("[main] server terminated: %v", err)
	}
}
