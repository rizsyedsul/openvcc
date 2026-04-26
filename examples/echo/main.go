package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var version = "dev"

type whoami struct {
	Cloud    string    `json:"cloud,omitempty"`
	Region   string    `json:"region,omitempty"`
	Hostname string    `json:"hostname"`
	Version  string    `json:"version"`
	Now      time.Time `json:"now"`
}

func main() {
	listen := flag.String("listen", envOr("LISTEN", ":8000"), "listen address")
	cloud := flag.String("cloud", os.Getenv("CLOUD"), "cloud label (aws, azure, ...)")
	region := flag.String("region", os.Getenv("REGION"), "region label")
	flag.Parse()

	hostname, _ := os.Hostname()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, whoami{
			Cloud:    *cloud,
			Region:   *region,
			Hostname: hostname,
			Version:  version,
			Now:      time.Now().UTC(),
		})
	})
	mux.HandleFunc("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, whoami{
			Cloud:    *cloud,
			Region:   *region,
			Hostname: hostname,
			Version:  version,
			Now:      time.Now().UTC(),
		})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("echo listening on %s (cloud=%s region=%s)", *listen, *cloud, *region)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
