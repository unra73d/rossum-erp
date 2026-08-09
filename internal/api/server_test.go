package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/unra73d/rossum-erp/internal/store"
)

func testServer(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "erpx.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return New(st, nil, slog.New(slog.DiscardHandler))
}

func do(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := map[string]any{}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: response is not JSON object: %s", method, path, rec.Body.String())
		}
	}
	return rec.Code, out
}

func TestVendorsSeeded(t *testing.T) {
	h := testServer(t)
	status, body := do(t, h, "GET", "/api/vendors", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if body["count"].(float64) != 4 {
		t.Errorf("seeded %v vendors, want 4", body["count"])
	}
}

func TestVendorCRUD(t *testing.T) {
	h := testServer(t)

	status, _ := do(t, h, "POST", "/api/vendors", `{"code":"7001","name":"New Vendor","vat_number":"CZ99"}`)
	if status != http.StatusCreated {
		t.Fatalf("create status = %d", status)
	}
	if status, _ := do(t, h, "POST", "/api/vendors", `{"code":"7001","name":"Dup"}`); status != http.StatusConflict {
		t.Errorf("duplicate create status = %d, want 409", status)
	}

	status, body := do(t, h, "PUT", "/api/vendors/7001", `{"name":"Renamed Vendor"}`)
	if status != http.StatusOK || body["name"] != "Renamed Vendor" {
		t.Errorf("update status = %d body = %v", status, body)
	}
	if body["code"] != "7001" {
		t.Errorf("update dropped the code: %v", body["code"])
	}

	if status, _ := do(t, h, "DELETE", "/api/vendors/7001", ""); status != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", status)
	}
	if status, _ := do(t, h, "GET", "/api/vendors/7001", ""); status != http.StatusNotFound {
		t.Errorf("get after delete status = %d, want 404", status)
	}
}

func TestMatchEndpoint(t *testing.T) {
	h := testServer(t)

	status, body := do(t, h, "GET", "/api/vendors/match?vat=CZ34560613&name=Supplier+s.r.o.", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %v", status, body)
	}
	matches := body["matches"].([]any)
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if code := matches[0].(map[string]any)["code"]; code != "9999" {
		t.Errorf("code = %v, want 9999", code)
	}

	// A lookup that resolves to nothing is still 200 with an empty array, so
	// the caller reads the body instead of handling a transport error.
	status, body = do(t, h, "GET", "/api/vendors/match?name=Unknown+Corp", "")
	if status != http.StatusOK {
		t.Fatalf("miss status = %d, want 200", status)
	}
	matches, ok := body["matches"].([]any)
	if !ok {
		t.Fatal("matches missing; callers expect an array even when empty")
	}
	if len(matches) != 0 {
		t.Errorf("got %d matches, want 0", len(matches))
	}
}

func TestPostInvoiceFlat(t *testing.T) {
	h := testServer(t)
	payload := `{
		"vendor_code":"9999","number":"202201","issue_date":"2026-07-01","due_date":"2099-01-01",
		"net_total":120000,"supplier_vat_number":"CZ34560613",
		"line_items":[{"description":"product 1","quantity":1,"unit_price_base":120000,"amount_total_base":120000}]
	}`

	status, body := do(t, h, "POST", "/api/invoices", payload)
	if status != http.StatusCreated {
		t.Fatalf("status = %d body = %v", status, body)
	}
	if body["status"] != "posted" {
		t.Errorf("status field = %v", body["status"])
	}

	// Re-posting the same vendor and number is a duplicate, not a second
	// document; Rossum retries webhooks and must not double-book.
	if status, _ := do(t, h, "POST", "/api/invoices", payload); status != http.StatusConflict {
		t.Errorf("duplicate post status = %d, want 409", status)
	}
}

func TestPostInvoiceRejectedAndLogged(t *testing.T) {
	h := testServer(t)
	status, body := do(t, h, "POST", "/api/invoices",
		`{"vendor_code":"0000","number":"X","issue_date":"2026-07-01","net_total":10,"supplier_vat_number":"CZ-1"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", status)
	}
	if body["error"] != "validation_failed" {
		t.Errorf("error = %v", body["error"])
	}

	// The rejection must be visible in the event log for troubleshooting.
	_, events := do(t, h, "GET", "/api/events", "")
	results := events["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("logged %d events, want 1", len(results))
	}
	logged := results[0].(map[string]any)
	if logged["status"].(float64) != 422 || logged["format"] != "flat" {
		t.Errorf("event = %v", logged)
	}
}

func TestPostInvoiceRossumPayload(t *testing.T) {
	h := testServer(t)
	payload := `{
	  "annotation": {"id": 42, "url": "https://api.elis.rossum.ai/v1/annotations/42", "content": [
	    {"schema_id":"basic_info_section","category":"section","children":[
	      {"schema_id":"document_id","category":"datapoint","content":{"value":"202299"}},
	      {"schema_id":"date_issue","category":"datapoint","content":{"value":"1/7/2026","normalized_value":"2026-07-01"}},
	      {"schema_id":"vendor_code","category":"datapoint","content":{"value":"9999"}},
	      {"schema_id":"amount_total_base","category":"datapoint","content":{"value":"120000"}}
	    ]},
	    {"schema_id":"line_items_section","category":"section","children":[
	      {"schema_id":"line_items","category":"multivalue","children":[
	        {"schema_id":"line_item","category":"tuple","children":[
	          {"schema_id":"item_description","category":"datapoint","content":{"value":"product 1"}},
	          {"schema_id":"item_quantity","category":"datapoint","content":{"value":"1"}},
	          {"schema_id":"item_amount_base","category":"datapoint","content":{"value":"120000"}},
	          {"schema_id":"item_amount_total_base","category":"datapoint","content":{"value":"120000"}}
	        ]}
	      ]}
	    ]}
	  ]},
	  "document": {"file_name":"invoice_1.pdf"}
	}`

	status, body := do(t, h, "POST", "/api/invoices", payload)
	if status != http.StatusCreated {
		t.Fatalf("status = %d body = %v", status, body)
	}
	inv := body["invoice"].(map[string]any)
	if inv["number"] != "202299" || inv["vendor_code"] != "9999" {
		t.Errorf("invoice = %v", inv)
	}
	if inv["annotation_id"] != "42" {
		t.Errorf("annotation_id = %v, want 42", inv["annotation_id"])
	}

	_, events := do(t, h, "GET", "/api/events", "")
	logged := events["results"].([]any)[0].(map[string]any)
	if logged["format"] != "rossum" {
		t.Errorf("event format = %v, want rossum", logged["format"])
	}
}
