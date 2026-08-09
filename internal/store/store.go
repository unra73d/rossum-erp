// Package store is a tiny mutex-guarded in-memory database that snapshots
// itself to a JSON file. It stands in for the ERPX persistence layer; the
// point of this service is the API contract, not the storage engine.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/unra73d/rossum-erp/internal/model"
	"github.com/unra73d/rossum-erp/internal/seed"
)

// ErrNotFound is returned when a lookup by id or code misses.
var ErrNotFound = errors.New("not found")

const maxEvents = 200

type snapshot struct {
	Vendors  []model.Vendor  `json:"vendors"`
	Invoices []model.Invoice `json:"invoices"`
	Events   []model.Event   `json:"events"`
	Seq      int             `json:"seq"`
}

// Store holds all ERPX state.
type Store struct {
	mu       sync.RWMutex
	path     string
	vendors  map[string]model.Vendor
	invoices map[string]model.Invoice
	events   []model.Event
	seq      int
}

// Open loads the snapshot at path, falling back to the embedded vendor seed
// when the file is missing or holds no vendors.
func Open(path string) (*Store, error) {
	s := &Store{
		path:     path,
		vendors:  map[string]model.Vendor{},
		invoices: map[string]model.Invoice{},
	}

	if data, err := os.ReadFile(path); err == nil {
		var snap snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, v := range snap.Vendors {
			s.vendors[v.Code] = v
		}
		for _, inv := range snap.Invoices {
			s.invoices[inv.ID] = inv
		}
		s.events = snap.Events
		s.seq = snap.Seq
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if len(s.vendors) == 0 {
		var seeded []model.Vendor
		if err := json.Unmarshal(seed.Vendors, &seeded); err != nil {
			return nil, fmt.Errorf("parse vendor seed: %w", err)
		}
		now := time.Now().UTC()
		for _, v := range seeded {
			v.UpdatedAt = now
			s.vendors[v.Code] = v
		}
		if err := s.persist(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// persist writes the snapshot. Callers must hold the write lock.
func (s *Store) persist() error {
	if s.path == "" {
		return nil
	}
	snap := snapshot{Events: s.events, Seq: s.seq}
	for _, v := range s.vendors {
		snap.Vendors = append(snap.Vendors, v)
	}
	for _, inv := range s.invoices {
		snap.Invoices = append(snap.Invoices, inv)
	}
	sort.Slice(snap.Vendors, func(i, j int) bool { return snap.Vendors[i].Code < snap.Vendors[j].Code })
	sort.Slice(snap.Invoices, func(i, j int) bool { return snap.Invoices[i].ID < snap.Invoices[j].ID })

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(s.path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) nextID(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s-%04d", prefix, s.seq)
}

// --- vendors ---------------------------------------------------------------

// Vendors returns every vendor ordered by code.
func (s *Store) Vendors() []model.Vendor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Vendor, 0, len(s.vendors))
	for _, v := range s.vendors {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Vendor looks a vendor up by its ERPX code.
func (s *Store) Vendor(code string) (model.Vendor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.vendors[code]
	if !ok {
		return model.Vendor{}, ErrNotFound
	}
	return v, nil
}

// SaveVendor inserts or replaces a vendor.
func (s *Store) SaveVendor(v model.Vendor) (model.Vendor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.UpdatedAt = time.Now().UTC()
	s.vendors[v.Code] = v
	return v, s.persist()
}

// DeleteVendor removes a vendor by code.
func (s *Store) DeleteVendor(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.vendors[code]; !ok {
		return ErrNotFound
	}
	delete(s.vendors, code)
	return s.persist()
}

// --- invoices --------------------------------------------------------------

// Invoices returns every invoice, newest first.
func (s *Store) Invoices() []model.Invoice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Invoice, 0, len(s.invoices))
	for _, inv := range s.invoices {
		out = append(out, inv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt.After(out[j].ReceivedAt) })
	return out
}

// Invoice looks an invoice up by id.
func (s *Store) Invoice(id string) (model.Invoice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inv, ok := s.invoices[id]
	if !ok {
		return model.Invoice{}, ErrNotFound
	}
	return inv, nil
}

// FindInvoice returns the existing invoice with this vendor and number, which
// is how ERPX detects a duplicate posting.
func (s *Store) FindInvoice(vendorCode, number string) (model.Invoice, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, inv := range s.invoices {
		if inv.VendorCode == vendorCode && inv.Number == number {
			return inv, true
		}
	}
	return model.Invoice{}, false
}

// CreateInvoice stores a new invoice and assigns it an ERPX document id.
func (s *Store) CreateInvoice(inv model.Invoice) (model.Invoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv.ID = s.nextID("INV")
	inv.ReceivedAt = time.Now().UTC()
	s.invoices[inv.ID] = inv
	return inv, s.persist()
}

// UpdateInvoice replaces an existing invoice, keeping its id and receipt time.
func (s *Store) UpdateInvoice(id string, inv model.Invoice) (model.Invoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.invoices[id]
	if !ok {
		return model.Invoice{}, ErrNotFound
	}
	inv.ID = id
	inv.ReceivedAt = old.ReceivedAt
	s.invoices[id] = inv
	return inv, s.persist()
}

// DeleteInvoice removes an invoice by id.
func (s *Store) DeleteInvoice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.invoices[id]; !ok {
		return ErrNotFound
	}
	delete(s.invoices, id)
	return s.persist()
}

// --- events ----------------------------------------------------------------

// Events returns the recent webhook calls, newest first.
func (s *Store) Events() []model.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Event, len(s.events))
	copy(out, s.events)
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// ClearEvents empties the event log.
func (s *Store) ClearEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = nil
	_ = s.persist()
}

// LogEvent appends an inbound call to the capped event log.
func (s *Store) LogEvent(e model.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID = s.nextID("EVT")
	e.At = time.Now().UTC()
	s.events = append(s.events, e)
	if len(s.events) > maxEvents {
		s.events = s.events[len(s.events)-maxEvents:]
	}
	_ = s.persist()
}
