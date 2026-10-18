// Package server wires the HTTP surface of the monolith: the JSON API, the
// media proxy for Garage objects, and the embedded React app.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/devileya/my-portfolio/internal/auth"
	"github.com/devileya/my-portfolio/internal/config"
	"github.com/devileya/my-portfolio/internal/content"
	"github.com/devileya/my-portfolio/internal/jobs"
	"github.com/devileya/my-portfolio/internal/storage"
)

type Server struct {
	cfg   config.Config
	repo  *content.Repo
	auth  *auth.Service
	store *storage.Store
	jobs  *jobs.Service
	web   fs.FS
	index []byte

	loginThrottle   *auth.Throttle
	messageThrottle *auth.Throttle
}

func New(cfg config.Config, repo *content.Repo, authSvc *auth.Service, store *storage.Store, jobsSvc *jobs.Service, web fs.FS) (*Server, error) {
	index, err := fs.ReadFile(web, "index.html")
	if err != nil {
		return nil, errors.New("web/dist/index.html missing: run `make web` before building the server")
	}
	return &Server{
		cfg: cfg, repo: repo, auth: authSvc, store: store, jobs: jobsSvc, web: web, index: index,
		loginThrottle:   auth.NewThrottle(8, 15*time.Minute),
		messageThrottle: auth.NewThrottle(5, 10*time.Minute),
	}, nil
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID, middleware.Recoverer, requestLog, securityHeaders)

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.NoCache)
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "storage": s.store != nil})
		})
		r.Get("/site", s.getSite)
		r.Get("/cv", s.downloadCV)
		r.Post("/messages", s.postMessage)

		r.Route("/admin", func(r chi.Router) {
			r.Post("/login", s.login)
			r.Post("/logout", s.logout)
			r.Group(func(r chi.Router) {
				r.Use(s.auth.Require)
				s.adminRoutes(r)
			})
		})
		r.NotFound(func(w http.ResponseWriter, r *http.Request) { writeError(w, http.StatusNotFound, "No such endpoint.") })
	})

	r.Get("/media/*", s.serveMedia)
	r.Handle("/*", http.HandlerFunc(s.serveApp))
	return r
}

// serveApp serves built assets, and index.html for everything else. Under
// ADMIN_PATH the page is told where the dashboard lives through a meta tag,
// so the public bundle never contains that path.
func (s *Server) serveApp(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p != "" && p != "index.html" {
		if f, err := fs.Stat(s.web, p); err == nil && !f.IsDir() {
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=3600")
			}
			http.ServeFileFS(w, r, s.web, p)
			return
		}
	}

	isAdmin := r.URL.Path == s.cfg.AdminPath || strings.HasPrefix(r.URL.Path, s.cfg.AdminPath+"/")
	status := http.StatusOK
	if !isAdmin && r.URL.Path != "/" {
		status = http.StatusNotFound
	}

	head := s.headTags(r.Context(), isAdmin)
	page := bytes.Replace(s.index, []byte("<!--pf:head-->"), []byte(head), 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if isAdmin {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	}
	w.WriteHeader(status)
	_, _ = w.Write(page)
}

func (s *Server) headTags(ctx context.Context, isAdmin bool) string {
	var b strings.Builder
	if isAdmin {
		b.WriteString(`<meta name="pf-admin-base" content="` + html.EscapeString(s.cfg.AdminPath) + `">`)
		b.WriteString(`<meta name="robots" content="noindex, nofollow"><title>Dashboard</title>`)
		return b.String()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	p, err := s.repo.Profile(ctx)
	if err != nil || p.Name == "" {
		b.WriteString(`<title>Portfolio</title>`)
		return b.String()
	}
	title := p.Name
	if p.Role != "" {
		title += " — " + p.Role
	}
	desc := p.Summary
	if len(desc) > 200 {
		desc = desc[:197] + "..."
	}
	b.WriteString(`<title>` + html.EscapeString(title) + `</title>`)
	b.WriteString(`<meta name="description" content="` + html.EscapeString(desc) + `">`)
	b.WriteString(`<meta property="og:title" content="` + html.EscapeString(title) + `">`)
	b.WriteString(`<meta property="og:description" content="` + html.EscapeString(desc) + `">`)
	b.WriteString(`<meta property="og:type" content="website">`)
	if p.AvatarURL != "" {
		b.WriteString(`<meta property="og:image" content="` + html.EscapeString(s.cfg.PublicURL+p.AvatarURL) + `">`)
	}
	return b.String()
}

// serveMedia streams an object from Garage. Keys are content-addressed, so
// responses are immutable.
func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	if key == "" || strings.Contains(key, "..") || !strings.HasPrefix(key, "images/") {
		http.NotFound(w, r)
		return
	}
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm == `"`+key+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	obj, err := s.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("media fetch failed", "key", key, "err", err)
		http.Error(w, "image unavailable", http.StatusBadGateway)
		return
	}
	defer obj.Close()
	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+key+`"`)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = io.Copy(w, obj)
}

// ── middleware & helpers ─────────────────────────────────────────────────────

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if !strings.HasPrefix(r.URL.Path, "/media/") {
			h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; "+
				"font-src 'self' data:; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		}
		next.ServeHTTP(w, r)
	})
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(),
			"ms", time.Since(start).Milliseconds())
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "The request body is not valid JSON.")
		return false
	}
	return true
}
