// Package api exposes the mock ERPX HTTP surface: vendor master data, the
// invoice posting webhook Rossum calls, and a log of inbound calls.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/unra73d/rossum-erp/internal/model"
	"github.com/unra73d/rossum-erp/internal/rossum"
	"github.com/unra73d/rossum-erp/internal/store"
)

// maxBodyBytes caps an inbound webhook body. Rossum annotation payloads with
// many line items are large but nowhere near this.
const maxBodyBytes = 8 << 20

// Server wires the store and the embedded UI into a handler.
type Server struct {
	store *store.Store
	ui    fs.FS
	log   *slog.Logger
}

// New builds the ERPX handler. ui may be nil when running API-only.
func New(st *store.Store, ui fs.FS, log *slog.Logger) http.Handler {
	s := &Server{store: st, ui: ui, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)

	mux.HandleFunc("GET /api/vendors", s.listVendors)
	mux.HandleFunc("GET /api/vendors/match", s.matchVendor)
	mux.HandleFunc("POST /api/vendors", s.createVendor)
	mux.HandleFunc("GET /api/vendors/{code}", s.getVendor)
	mux.HandleFunc("PUT /api/vendors/{code}", s.updateVendor)
	mux.HandleFunc("DELETE /api/vendors/{code}", s.deleteVendor)

	mux.HandleFunc("GET /api/invoices", s.listInvoices)
	mux.HandleFunc("POST /api/invoices", s.postInvoice)
	mux.HandleFunc("GET /api/invoices/{id}", s.getInvoice)
	mux.HandleFunc("PUT /api/invoices/{id}", s.updateInvoice)
	mux.HandleFunc("DELETE /api/invoices/{id}", s.deleteInvoice)

	mux.HandleFunc("GET /api/events", s.listEvents)
	mux.HandleFunc("DELETE /api/events", s.clearEvents)

	if ui != nil {
		mux.Handle("/", s.spa())
	}
	return withCORS(mux)
}

// withCORS lets the Rossum UI extension and local tooling call this API from a
// browser. The mock ERP is deliberately unauthenticated - see README.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// spa serves the built Svelte app, falling back to index.html so client-side
// routes survive a page reload.
func (s *Server) spa() http.Handler {
	files := http.FileServer(http.FS(s.ui))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(s.ui, path); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
}

// --- vendors ---------------------------------------------------------------

func (s *Server) listVendors(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	vendors := s.store.Vendors()
	if q != "" {
		filtered := vendors[:0:0]
		for _, v := range vendors {
			hay := strings.ToLower(strings.Join([]string{v.Code, v.Name, v.VATNumber, v.City, v.Country}, " "))
			if strings.Contains(hay, q) {
				filtered = append(filtered, v)
			}
		}
		vendors = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(vendors), "results": vendors})
}

func (s *Server) getVendor(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Vendor(r.PathValue("code"))
	if err != nil {
		writeError(w, http.StatusNotFound, "vendor_not_found", "No vendor with that code.")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) createVendor(w http.ResponseWriter, r *http.Request) {
	var v model.Vendor
	if !decode(w, r, &v) {
		return
	}
	v.Code = strings.TrimSpace(v.Code)
	if v.Code == "" || strings.TrimSpace(v.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_vendor", "Both code and name are required.")
		return
	}
	if _, err := s.store.Vendor(v.Code); err == nil {
		writeError(w, http.StatusConflict, "vendor_exists", "A vendor with that code already exists.")
		return
	}
	saved, err := s.store.SaveVendor(v)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) updateVendor(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if _, err := s.store.Vendor(code); err != nil {
		writeError(w, http.StatusNotFound, "vendor_not_found", "No vendor with that code.")
		return
	}
	var v model.Vendor
	if !decode(w, r, &v) {
		return
	}
	v.Code = code
	if strings.TrimSpace(v.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_vendor", "Name is required.")
		return
	}
	saved, err := s.store.SaveVendor(v)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) deleteVendor(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteVendor(r.PathValue("code")); err != nil {
		writeError(w, http.StatusNotFound, "vendor_not_found", "No vendor with that code.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// matchVendor is the endpoint the Rossum vendor-matching function calls. It
// never 404s on a miss: a miss is a normal, expected answer that the caller
// turns into a blocking message on the annotation.
func (s *Server) matchVendor(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result := MatchVendor(s.store.Vendors(), MatchQuery{
		VAT:     q.Get("vat"),
		Name:    q.Get("name"),
		IBAN:    q.Get("iban"),
		Account: q.Get("account"),
		TaxNum:  q.Get("tax_number"),
	})
	writeJSON(w, http.StatusOK, result)
}

// --- invoices --------------------------------------------------------------

func (s *Server) listInvoices(w http.ResponseWriter, r *http.Request) {
	invoices := s.store.Invoices()
	if code := r.URL.Query().Get("vendor_code"); code != "" {
		filtered := invoices[:0:0]
		for _, inv := range invoices {
			if inv.VendorCode == code {
				filtered = append(filtered, inv)
			}
		}
		invoices = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(invoices), "results": invoices})
}

func (s *Server) getInvoice(w http.ResponseWriter, r *http.Request) {
	inv, err := s.store.Invoice(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice_not_found", "No invoice with that id.")
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// postInvoice is the webhook target. It accepts either a flat ERPX invoice or
// a raw Rossum annotation payload, validates it the way the real ERPX would,
// and records the call either way so failures are visible in the UI.
func (s *Server) postInvoice(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body is too large.")
		return
	}

	event := model.Event{Method: r.Method, Path: r.URL.Path, RawRequest: string(body)}
	finish := func(status int, codes []string, invoiceID string) {
		event.Status = status
		event.Errors = codes
		event.InvoiceID = invoiceID
		s.store.LogEvent(event)
	}

	var inv model.Invoice
	if rossum.Looks(body) {
		event.Format = "rossum"
		parsed, err := rossum.Parse(body)
		if err != nil {
			finish(http.StatusBadRequest, []string{"payload_invalid"}, "")
			writeError(w, http.StatusBadRequest, "payload_invalid", err.Error())
			return
		}
		inv = parsed
	} else {
		event.Format = "flat"
		if err := json.Unmarshal(body, &inv); err != nil {
			finish(http.StatusBadRequest, []string{"payload_invalid"}, "")
			writeError(w, http.StatusBadRequest, "payload_invalid", "Body is not valid JSON: "+err.Error())
			return
		}
	}

	vendorExists := func(code string) bool {
		_, err := s.store.Vendor(code)
		return err == nil
	}
	errs, warns := Validate(inv, vendorExists, today())
	if len(errs) > 0 {
		codes := make([]string, len(errs))
		for i, e := range errs {
			codes[i] = e.Code
		}
		finish(http.StatusUnprocessableEntity, codes, "")
		s.log.Warn("rejected invoice", "number", inv.Number, "vendor", inv.VendorCode, "errors", codes)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":    "validation_failed",
			"message":  "ERPX refused the document.",
			"errors":   errs,
			"warnings": warns,
		})
		return
	}

	if existing, dup := s.store.FindInvoice(inv.VendorCode, inv.Number); dup {
		finish(http.StatusConflict, []string{"duplicate_invoice"}, existing.ID)
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "duplicate_invoice",
			"message": "This vendor and invoice number are already posted in ERPX.",
			"id":      existing.ID,
		})
		return
	}

	saved, err := s.store.CreateInvoice(inv)
	if err != nil {
		finish(http.StatusInternalServerError, []string{"storage_error"}, "")
		s.fail(w, err)
		return
	}
	finish(http.StatusCreated, nil, saved.ID)
	s.log.Info("posted invoice", "id", saved.ID, "number", saved.Number, "vendor", saved.VendorCode)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       saved.ID,
		"status":   "posted",
		"invoice":  saved,
		"warnings": warns,
	})
}

func (s *Server) updateInvoice(w http.ResponseWriter, r *http.Request) {
	var inv model.Invoice
	if !decode(w, r, &inv) {
		return
	}
	saved, err := s.store.UpdateInvoice(r.PathValue("id"), inv)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "invoice_not_found", "No invoice with that id.")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) deleteInvoice(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteInvoice(r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "invoice_not_found", "No invoice with that id.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- events ----------------------------------------------------------------

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	events := s.store.Events()
	writeJSON(w, http.StatusOK, map[string]any{"count": len(events), "results": events})
}

func (s *Server) clearEvents(w http.ResponseWriter, r *http.Request) {
	s.store.ClearEvents()
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---------------------------------------------------------------

// today is the reference date for the due-date rule, in UTC and truncated to
// midnight so an invoice due today is still acceptable.
func today() time.Time {
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body is too large.")
		return false
	}
	if err := json.Unmarshal(body, dst); err != nil {
		writeError(w, http.StatusBadRequest, "payload_invalid", "Body is not valid JSON: "+err.Error())
		return false
	}
	return true
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong on the ERPX side.")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": code, "message": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("encode response", "error", err)
	}
}
