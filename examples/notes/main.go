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

	_ "github.com/lib/pq"
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
	cloud := flag.String("cloud", os.Getenv("CLOUD"), "cloud label")
	region := flag.String("region", os.Getenv("REGION"), "region label")
	dsn := flag.String("dsn", os.Getenv("DSN"), "CockroachDB / Postgres DSN")
	flag.Parse()

	if *dsn == "" {
		log.Fatal("dsn is required (set DSN env or --dsn)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := openStore(ctx, *dsn)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	hostname, _ := os.Hostname()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := store.Health(r.Context()); err != nil {
			http.Error(w, "db unhealthy: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, whoami{
			Cloud: *cloud, Region: *region, Hostname: hostname,
			Version: version, Now: time.Now().UTC(),
		})
	})

	mux.HandleFunc("POST /notes", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if in.Text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		n, err := store.Insert(r.Context(), in.Text, *cloud)
		if err != nil {
			http.Error(w, "insert: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, n)
	})

	mux.HandleFunc("GET /notes", func(w http.ResponseWriter, r *http.Request) {
		notes, err := store.List(r.Context(), 100)
		if err != nil {
			http.Error(w, "list: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, notes)
	})

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("notes listening on %s (cloud=%s region=%s)", *listen, *cloud, *region)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
