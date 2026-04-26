package admin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/syedsumx/openvcc/internal/pool"
	"github.com/syedsumx/openvcc/internal/proxy"
)

type Options struct {
	Token  string
	Reload func(ctx context.Context) error
}

type Server struct {
	pool   *pool.Pool
	proxy  *proxy.Handler
	log    *slog.Logger
	token  string
	reload func(ctx context.Context) error
}

func New(p *pool.Pool, ph *proxy.Handler, log *slog.Logger, opts Options) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		pool:   p,
		proxy:  ph,
		log:    log.With("component", "admin"),
		token:  opts.Token,
		reload: opts.Reload,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/backends", s.listBackends)
	mux.HandleFunc("POST /admin/backends", s.addBackend)
	mux.HandleFunc("DELETE /admin/backends/{name}", s.removeBackend)
	mux.HandleFunc("PUT /admin/backends/{name}/health", s.setBackendHealth)
	mux.HandleFunc("POST /admin/reload", s.reloadConfig)
	return s.auth(mux)
}

func (s *Server) setBackendHealth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var in struct {
		Healthy *bool `json:"healthy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	if in.Healthy == nil {
		writeError(w, http.StatusBadRequest, errors.New("healthy is required"))
		return
	}
	b, ok := s.pool.Find(name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("backend %q not found", name))
		return
	}
	b.SetHealthy(*in.Healthy)
	s.log.Info("backend health overridden via admin", "backend", name, "healthy", *in.Healthy)
	writeJSON(w, http.StatusOK, toDTO(b))
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			http.Error(w, "admin disabled: no bearer token configured", http.StatusForbidden)
			return
		}
		hdr := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(hdr, prefix) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="openvcc"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		got := hdr[len(prefix):]
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="openvcc"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type backendDTO struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Cloud    string `json:"cloud,omitempty"`
	Region   string `json:"region,omitempty"`
	Weight   int    `json:"weight,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Healthy  bool   `json:"healthy"`
	Inflight int64  `json:"inflight"`
}

func toDTO(b *pool.Backend) backendDTO {
	return backendDTO{
		Name:     b.Name,
		URL:      b.URL.String(),
		Cloud:    b.Cloud,
		Region:   b.Region,
		Weight:   b.Weight,
		Kind:     b.Kind,
		Healthy:  b.Healthy(),
		Inflight: b.Inflight(),
	}
}

func (s *Server) listBackends(w http.ResponseWriter, r *http.Request) {
	bs := s.pool.Backends()
	out := make([]backendDTO, 0, len(bs))
	for _, b := range bs {
		out = append(out, toDTO(b))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) addBackend(w http.ResponseWriter, r *http.Request) {
	var in backendDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	if in.Name == "" || in.URL == "" {
		writeError(w, http.StatusBadRequest, errors.New("name and url are required"))
		return
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeError(w, http.StatusBadRequest, errors.New("url must be http(s) with a host"))
		return
	}
	if in.Weight <= 0 {
		in.Weight = 1
	}
	b := pool.NewBackend(in.Name, u, in.Cloud, in.Region, in.Weight).WithKind(in.Kind)
	if !s.pool.Add(b) {
		writeError(w, http.StatusConflict, fmt.Errorf("backend %q already exists", in.Name))
		return
	}
	s.log.Info("backend added via admin", "backend", in.Name, "url", in.URL)
	writeJSON(w, http.StatusCreated, toDTO(b))
}

func (s *Server) removeBackend(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.pool.Remove(name) {
		writeError(w, http.StatusNotFound, fmt.Errorf("backend %q not found", name))
		return
	}
	if s.proxy != nil {
		s.proxy.Forget(name)
	}
	s.log.Info("backend removed via admin", "backend", name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reloadConfig(w http.ResponseWriter, r *http.Request) {
	if s.reload == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("reload not configured"))
		return
	}
	if err := s.reload(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.log.Info("config reloaded via admin")
	writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
