// Package api exposes the rates database over HTTP as JSON.
package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/melastore/birrwatch/internal/store"
)

// Server wires the store to HTTP handlers.
type Server struct {
	store  *store.Store
	log    *slog.Logger
	webDir string
}

// New returns a Server. webDir may be empty, in which case only the API is
// served.
func New(s *store.Store, log *slog.Logger, webDir string) *Server {
	return &Server{store: s, log: log, webDir: webDir}
}

// Routes builds the HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/currencies", s.handleCurrencies)
	mux.HandleFunc("GET /api/rates", s.handleRates)
	mux.HandleFunc("GET /api/spread", s.handleSpread)

	if s.webDir != "" {
		mux.Handle("/", s.spaHandler())
	}
	return withCORS(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCurrencies(w http.ResponseWriter, r *http.Request) {
	codes, err := s.store.Currencies(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, codes)
}

func (s *Server) handleRates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	currency, err := cleanCurrency(q.Get("currency"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	from, err := cleanDate(q.Get("from"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "from: "+err.Error())
		return
	}
	to, err := cleanDate(q.Get("to"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "to: "+err.Error())
		return
	}

	limit := 1000
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 10000 {
			writeErr(w, http.StatusBadRequest, "limit must be between 1 and 10000")
			return
		}
		limit = n
	}

	rates, err := s.store.Rates(r.Context(), store.RatesQuery{
		Source:   q.Get("source"),
		Currency: currency,
		From:     from,
		To:       to,
		Limit:    limit,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rates)
}

func (s *Server) handleSpread(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	currency, err := cleanCurrency(q.Get("currency"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if currency == "" {
		currency = "USD"
	}
	from, err := cleanDate(q.Get("from"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "from: "+err.Error())
		return
	}
	to, err := cleanDate(q.Get("to"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "to: "+err.Error())
		return
	}

	points, err := s.store.Spread(r.Context(), currency, from, to)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, points)
}

// spaHandler serves the built dashboard, falling back to index.html so client
// routes survive a page refresh.
func (s *Server) spaHandler() http.Handler {
	fileServer := http.FileServer(http.Dir(s.webDir))
	index := filepath.Join(s.webDir, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Clean(r.URL.Path)
		target := filepath.Join(s.webDir, clean)

		// Reject anything that escapes the web root.
		if rel, err := filepath.Rel(s.webDir, target); err != nil || strings.HasPrefix(rel, "..") {
			http.NotFound(w, r)
			return
		}
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := os.Stat(index); errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func cleanCurrency(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	if !currencyPattern.MatchString(s) {
		return "", errors.New("currency must be a 3-letter ISO 4217 code")
	}
	return s, nil
}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func cleanDate(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	s = strings.TrimSpace(s)
	if !datePattern.MatchString(s) {
		return "", errors.New("must be YYYY-MM-DD")
	}
	return s, nil
}

// fail logs the real error and returns a generic message, so internal details
// never reach the client.
func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Error("request failed", "err", err)
	writeErr(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// withCORS allows the Vite dev server to call the API cross-origin. The data is
// public, so a permissive read-only policy costs nothing.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
