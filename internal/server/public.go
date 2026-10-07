package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/devileya/my-portfolio/internal/storage"
)

func (s *Server) getSite(w http.ResponseWriter, r *http.Request) {
	site, err := s.repo.Site(r.Context())
	if err != nil {
		slog.Error("load site", "err", err)
		writeError(w, http.StatusInternalServerError, "The site content could not be loaded.")
		return
	}
	s.repo.Increment(context.WithoutCancel(r.Context()), "site_views")
	writeJSON(w, http.StatusOK, site)
}

// downloadCV streams the uploaded CV as an attachment and counts it.
func (s *Server) downloadCV(w http.ResponseWriter, r *http.Request) {
	key, filename, err := s.repo.CVKey(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "The CV could not be loaded.")
		return
	}
	if key == "" {
		writeError(w, http.StatusNotFound, "No CV has been uploaded yet.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	obj, err := s.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "The CV file is missing from storage.")
			return
		}
		slog.Error("cv fetch failed", "err", err)
		writeError(w, http.StatusBadGateway, "The CV is temporarily unavailable. Try again in a minute.")
		return
	}
	defer obj.Close()

	if filename == "" {
		filename = "cv.pdf"
	}
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if r.Method != http.MethodHead && disposition == "attachment" {
		s.repo.Increment(context.WithoutCancel(r.Context()), "cv_downloads")
	}
	_, _ = io.Copy(w, obj)
}

type messageInput struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
	// Website is a honeypot: hidden from people, filled in by bots.
	Website string `json:"website"`
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request) {
	var in messageInput
	if !readJSON(w, r, &in) {
		return
	}
	if in.Website != "" {
		// Pretend success so the bot learns nothing.
		writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(in.Email)
	in.Message = strings.TrimSpace(in.Message)

	switch {
	case in.Name == "" || len(in.Name) > 120:
		writeError(w, http.StatusUnprocessableEntity, "Add your name (up to 120 characters).")
		return
	case !validEmail(in.Email):
		writeError(w, http.StatusUnprocessableEntity, "Enter an email address I can reply to.")
		return
	case len(in.Message) < 10 || len(in.Message) > 5000:
		writeError(w, http.StatusUnprocessableEntity, "Write a message between 10 and 5,000 characters.")
		return
	}
	if !s.messageThrottle.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Too many messages from this network. Try again in a few minutes.")
		return
	}
	ua := r.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	if err := s.repo.CreateMessage(r.Context(), in.Name, in.Email, in.Message, clientIP(r), ua); err != nil {
		slog.Error("save message", "err", err)
		writeError(w, http.StatusInternalServerError, "Your message was not sent. Try again, or use the email link.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func validEmail(s string) bool {
	if len(s) > 254 {
		return false
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && strings.Contains(s[strings.LastIndex(s, "@"):], ".")
}
