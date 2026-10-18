package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/devileya/my-portfolio/internal/auth"
	"github.com/devileya/my-portfolio/internal/content"
	"github.com/devileya/my-portfolio/internal/storage"
)

const (
	maxImageBytes = 10 << 20
	maxCVBytes    = 20 << 20
)

func (s *Server) adminRoutes(r chi.Router) {
	r.Get("/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, auth.FromContext(r.Context()))
	})
	r.Put("/password", s.changePassword)
	r.Get("/stats", s.stats)

	r.Get("/profile", s.getProfile)
	r.Put("/profile", s.putProfile)

	r.Post("/uploads", s.uploadImage)
	r.Post("/cv", s.uploadCV)
	r.Delete("/cv", s.deleteCV)

	r.Get("/projects", list(s.repo.Projects))
	r.Post("/projects", save(s.repo.SaveProject, validateProject, false))
	r.Put("/projects/{id}", save(s.repo.SaveProject, validateProject, true))
	r.Delete("/projects/{id}", remove(s.repo.DeleteProject))
	r.Post("/projects/reorder", s.reorder("projects"))

	r.Get("/experiences", list(s.repo.Experiences))
	r.Post("/experiences", save(s.repo.SaveExperience, validateExperience, false))
	r.Put("/experiences/{id}", save(s.repo.SaveExperience, validateExperience, true))
	r.Delete("/experiences/{id}", remove(s.repo.DeleteExperience))
	r.Post("/experiences/reorder", s.reorder("experiences"))

	r.Get("/skills", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.repo.Skills(r.Context())
		respond(w, v, err)
	})
	r.Post("/skills", save(s.repo.SaveSkill, validateSkill, false))
	r.Put("/skills/{id}", save(s.repo.SaveSkill, validateSkill, true))
	r.Delete("/skills/{id}", remove(s.repo.DeleteSkill))
	r.Post("/skills/reorder", s.reorder("skills"))

	r.Get("/messages", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.repo.Messages(r.Context())
		respond(w, v, err)
	})
	r.Patch("/messages/{id}", s.markMessage)
	r.Delete("/messages/{id}", remove(s.repo.DeleteMessage))

	r.Route("/jobs", s.jobRoutes)
}

// ── session ──────────────────────────────────────────────────────────────────

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.loginThrottle.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Too many sign-in attempts. Wait 15 minutes and try again.")
		return
	}
	var in struct{ Email, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	a, err := s.auth.Login(r.Context(), in.Email, in.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "That email and password don't match.")
		return
	}
	if err != nil {
		slog.Error("login", "err", err)
		writeError(w, http.StatusInternalServerError, "Sign-in is unavailable right now.")
		return
	}
	if err := s.auth.SetSession(w, a); err != nil {
		writeError(w, http.StatusInternalServerError, "Sign-in is unavailable right now.")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.auth.ClearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct{ Current, Next string }
	if !readJSON(w, r, &in) {
		return
	}
	admin := auth.FromContext(r.Context())
	err := s.auth.ChangePassword(r.Context(), admin.ID, in.Current, in.Next)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnprocessableEntity, "Your current password is incorrect.")
	case err != nil && strings.Contains(err.Error(), "at least"):
		writeError(w, http.StatusUnprocessableEntity, "Use a new password of at least 10 characters.")
	case err != nil:
		slog.Error("change password", "err", err)
		writeError(w, http.StatusInternalServerError, "The password was not changed.")
	default:
		// Older sessions are now revoked; keep this one signed in.
		_ = s.auth.SetSession(w, admin)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	v, err := s.repo.Stats(r.Context())
	respond(w, v, err)
}

// ── profile & files ──────────────────────────────────────────────────────────

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := s.repo.Profile(r.Context())
	respond(w, p, err)
}

func (s *Server) putProfile(w http.ResponseWriter, r *http.Request) {
	var p content.Profile
	if !readJSON(w, r, &p) {
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		writeError(w, http.StatusUnprocessableEntity, "Name can't be empty.")
		return
	}
	if p.AvatarKey != "" && !strings.HasPrefix(p.AvatarKey, "images/") {
		writeError(w, http.StatusUnprocessableEntity, "The photo must be an uploaded image.")
		return
	}
	if err := s.repo.UpdateProfile(r.Context(), p); err != nil {
		slog.Error("update profile", "err", err)
		writeError(w, http.StatusInternalServerError, "The profile was not saved.")
		return
	}
	s.getProfile(w, r)
}

var imageTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
	"image/avif": ".avif",
}

// uploadImage stores an image in Garage and returns its key and URL. The
// type is sniffed from the bytes, never trusted from the client; SVG is
// refused because it can carry script.
func (s *Server) uploadImage(w http.ResponseWriter, r *http.Request) {
	data, name, ok := s.readUpload(w, r, maxImageBytes)
	if !ok {
		return
	}
	ct := sniffImage(data)
	ext, allowed := imageTypes[ct]
	if !allowed {
		writeError(w, http.StatusUnsupportedMediaType, "Upload a PNG, JPEG, WebP, GIF or AVIF image.")
		return
	}
	obj, err := s.store.Put(r.Context(), "images", ext, ct, data)
	if s.storageFailed(w, err, name) {
		return
	}
	writeJSON(w, http.StatusCreated, obj)
}

func (s *Server) uploadCV(w http.ResponseWriter, r *http.Request) {
	data, name, ok := s.readUpload(w, r, maxCVBytes)
	if !ok {
		return
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		writeError(w, http.StatusUnsupportedMediaType, "Upload the CV as a PDF.")
		return
	}
	obj, err := s.store.Put(r.Context(), "cv", ".pdf", "application/pdf", data)
	if s.storageFailed(w, err, name) {
		return
	}
	oldKey, _, _ := s.repo.CVKey(r.Context())
	filename := cleanFilename(name, "cv.pdf")
	if err := s.repo.SetCV(r.Context(), obj.Key, filename, obj.Size, obj.SHA256); err != nil {
		writeError(w, http.StatusInternalServerError, "The CV was uploaded but not saved. Try again.")
		return
	}
	if oldKey != "" && oldKey != obj.Key {
		if err := s.store.Delete(r.Context(), oldKey); err != nil {
			slog.Warn("delete old cv", "key", oldKey, "err", err)
		}
	}
	s.getProfile(w, r)
}

func (s *Server) deleteCV(w http.ResponseWriter, r *http.Request) {
	oldKey, _, err := s.repo.CVKey(r.Context())
	if err == nil {
		err = s.repo.SetCV(r.Context(), "", "", 0, "")
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "The CV was not removed.")
		return
	}
	if oldKey != "" {
		_ = s.store.Delete(r.Context(), oldKey)
	}
	s.getProfile(w, r)
}

func (s *Server) readUpload(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, string, bool) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "File storage isn't configured. Set S3_ENDPOINT, S3_ACCESS_KEY and S3_SECRET_KEY.")
		return nil, "", false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+1<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "That file is too large. The limit is "+strconv.FormatInt(limit>>20, 10)+" MB.")
		} else {
			writeError(w, http.StatusBadRequest, "Attach the file in a form field named \"file\".")
		}
		return nil, "", false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "The upload was interrupted. Try again.")
		return nil, "", false
	}
	if int64(len(data)) > limit {
		writeError(w, http.StatusRequestEntityTooLarge, "That file is too large. The limit is "+strconv.FormatInt(limit>>20, 10)+" MB.")
		return nil, "", false
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "That file is empty.")
		return nil, "", false
	}
	return data, header.Filename, true
}

func (s *Server) storageFailed(w http.ResponseWriter, err error, name string) bool {
	if err == nil {
		return false
	}
	slog.Error("storage put", "file", name, "err", err)
	if errors.Is(err, storage.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "File storage isn't configured.")
	} else {
		writeError(w, http.StatusBadGateway, "File storage didn't accept the upload. Check that Garage is reachable and try again.")
	}
	return true
}

func sniffImage(data []byte) string {
	if len(data) > 12 && string(data[4:8]) == "ftyp" && (string(data[8:12]) == "avif" || string(data[8:12]) == "avis") {
		return "image/avif"
	}
	return http.DetectContentType(data)
}

func cleanFilename(name, fallback string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if r == '.' || r == '-' || r == '_' || r == ' ' || r == '(' || r == ')' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == "." || !strings.HasSuffix(strings.ToLower(out), ".pdf") {
		return fallback
	}
	return out
}

// ── messages ─────────────────────────────────────────────────────────────────

func (s *Server) markMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Read bool `json:"read"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	err := s.repo.MarkMessage(r.Context(), id, in.Read)
	if errors.Is(err, content.ErrNotFound) {
		writeError(w, http.StatusNotFound, "That message no longer exists.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "The message was not updated.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reorder(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			IDs []int64 `json:"ids"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		if err := s.repo.Reorder(r.Context(), table, in.IDs); err != nil {
			slog.Error("reorder", "table", table, "err", err)
			writeError(w, http.StatusInternalServerError, "The new order was not saved.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ── generic CRUD helpers ─────────────────────────────────────────────────────

func list[T any](fn func(context.Context, bool) ([]T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := fn(r.Context(), false)
		respond(w, v, err)
	}
}

// save decodes T, validates it, and passes it to fn. On PUT the id comes from
// the URL and is written into the value through setID.
func save[T any, P interface {
	*T
	SetID(int64)
}](fn func(context.Context, T) (T, error), validate func(*T) string, update bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var v T
		if !readJSON(w, r, &v) {
			return
		}
		if update {
			id, ok := idParam(w, r)
			if !ok {
				return
			}
			P(&v).SetID(id)
		} else {
			P(&v).SetID(0)
		}
		if msg := validate(&v); msg != "" {
			writeError(w, http.StatusUnprocessableEntity, msg)
			return
		}
		out, err := fn(r.Context(), v)
		if errors.Is(err, content.ErrNotFound) {
			writeError(w, http.StatusNotFound, "That item no longer exists.")
			return
		}
		if err != nil {
			slog.Error("save", "err", err)
			writeError(w, http.StatusInternalServerError, "The changes were not saved.")
			return
		}
		status := http.StatusOK
		if !update {
			status = http.StatusCreated
		}
		writeJSON(w, status, out)
	}
}

func remove(fn func(context.Context, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		err := fn(r.Context(), id)
		if errors.Is(err, content.ErrNotFound) {
			writeError(w, http.StatusNotFound, "That item no longer exists.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "The item was not deleted.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		slog.Error("query", "err", err)
		writeError(w, http.StatusInternalServerError, "The data could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid id.")
		return 0, false
	}
	return id, true
}

// ── validation ───────────────────────────────────────────────────────────────

var projectCategories = map[string]bool{"ai": true, "mobile": true, "web": true, "backend": true}

func validateProject(p *content.Project) string {
	p.Title = strings.TrimSpace(p.Title)
	p.Category = strings.ToLower(strings.TrimSpace(p.Category))
	switch {
	case p.Title == "":
		return "Give the project a title."
	case !projectCategories[p.Category]:
		return "Category must be one of: ai, mobile, web, backend."
	case p.ImageKey != "" && !strings.HasPrefix(p.ImageKey, "images/"):
		return "The project image must be an uploaded image."
	}
	for _, u := range []string{p.LiveURL, p.AltURL, p.RepoURL} {
		if !validLink(u) {
			return "Links must start with https://, http:// or /."
		}
	}
	return ""
}

func validateExperience(e *content.Experience) string {
	e.Title = strings.TrimSpace(e.Title)
	if e.Title == "" {
		return "Add a role or degree title."
	}
	return ""
}

func validateSkill(sk *content.Skill) string {
	sk.Name = strings.TrimSpace(sk.Name)
	if sk.Name == "" {
		return "Name the skill."
	}
	return ""
}

func validLink(u string) bool {
	u = strings.TrimSpace(u)
	return u == "" || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") ||
		(strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//"))
}
