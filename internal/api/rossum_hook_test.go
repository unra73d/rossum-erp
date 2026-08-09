package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// hookPayload builds a minimal annotation_content body: a vendor_code
// datapoint plus optional sender_vat_id_sanitized/sender_name, each with a
// distinct numeric id so operations/messages can be asserted against a
// specific field.
func hookPayload(vat, name string, includeVendorCode bool) string {
	var fields []string
	if includeVendorCode {
		fields = append(fields, `{"id":100,"schema_id":"vendor_code","category":"datapoint","content":{"value":""}}`)
	}
	if vat != "" {
		fields = append(fields, fmt.Sprintf(`{"id":101,"schema_id":"sender_vat_id_sanitized","category":"datapoint","content":{"value":%q}}`, vat))
	}
	if name != "" {
		fields = append(fields, fmt.Sprintf(`{"id":102,"schema_id":"sender_name","category":"datapoint","content":{"value":%q}}`, name))
	}
	return fmt.Sprintf(`{
		"action": "changed", "event": "annotation_content",
		"annotation": {"id": 1, "content": [
			{"schema_id":"vendor_section","category":"section","children":[%s]}
		]}
	}`, strings.Join(fields, ","))
}

func TestVendorMatchHookCleanMatch(t *testing.T) {
	h := testServer(t)
	status, body := do(t, h, "POST", "/rossum/vendor-match", hookPayload("CZ34560613", "Supplier s.r.o.", true))
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %v", status, body)
	}

	ops := body["operations"].([]any)
	if len(ops) != 1 {
		t.Fatalf("got %d operations, want 1", len(ops))
	}
	op := ops[0].(map[string]any)
	if op["id"] != float64(100) {
		t.Errorf("operation id = %v (%T), want 100 (vendor_code's id)", op["id"], op["id"])
	}
	value := op["value"].(map[string]any)["content"].(map[string]any)["value"]
	if value != "9999" {
		t.Errorf("written vendor code = %v, want 9999", value)
	}

	messages := body["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["type"] != "info" {
		t.Errorf("messages = %v, want one info message", messages)
	}
}

func TestVendorMatchHookNoMatch(t *testing.T) {
	h := testServer(t)
	status, body := do(t, h, "POST", "/rossum/vendor-match", hookPayload("GB000000", "Nobody Ltd", true))
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %v", status, body)
	}

	messages := body["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["type"] != "error" {
		t.Fatalf("messages = %v, want one error message", messages)
	}

	// vendor_code must be cleared, not left stale from a previous run.
	ops := body["operations"].([]any)
	op := ops[0].(map[string]any)
	value := op["value"].(map[string]any)["content"].(map[string]any)["value"]
	if value != "" {
		t.Errorf("vendor code = %q, want cleared to empty on a miss", value)
	}
}

func TestVendorMatchHookAmbiguous(t *testing.T) {
	h := testServer(t)
	// Two vendors sharing a VAT number - broken master data, not a code bug.
	doCreateVendor(t, h, `{"code":"7001","name":"Alpha","vat_number":"CZ999"}`)
	doCreateVendor(t, h, `{"code":"7002","name":"Beta","vat_number":"CZ999"}`)

	status, body := do(t, h, "POST", "/rossum/vendor-match", hookPayload("CZ999", "", true))
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %v", status, body)
	}
	messages := body["messages"].([]any)
	msg := messages[0].(map[string]any)
	if msg["type"] != "error" || !strings.Contains(msg["content"].(string), "7001") || !strings.Contains(msg["content"].(string), "7002") {
		t.Errorf("message = %v, want an error naming both 7001 and 7002", msg)
	}
}

func TestVendorMatchHookMissingSchemaField(t *testing.T) {
	h := testServer(t)
	status, body := do(t, h, "POST", "/rossum/vendor-match", hookPayload("CZ34560613", "Supplier s.r.o.", false))
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %v", status, body)
	}
	if _, hasOps := body["operations"]; hasOps {
		t.Error("no vendor_code field to write to, should have no operations")
	}
	messages := body["messages"].([]any)
	if len(messages) != 1 || !strings.Contains(messages[0].(map[string]any)["content"].(string), "vendor_code") {
		t.Errorf("messages = %v, want one message naming the missing schema field", messages)
	}
}

func TestVendorMatchHookNoInput(t *testing.T) {
	h := testServer(t)
	status, body := do(t, h, "POST", "/rossum/vendor-match", hookPayload("", "", true))
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %v", status, body)
	}
	messages := body["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["type"] != "error" {
		t.Fatalf("messages = %v, want one error about missing input", messages)
	}
}

func TestVendorMatchHookLogsEvents(t *testing.T) {
	h := testServer(t)
	do(t, h, "POST", "/rossum/vendor-match", hookPayload("CZ34560613", "Supplier s.r.o.", true))
	do(t, h, "POST", "/rossum/vendor-match", hookPayload("GB000000", "Nobody Ltd", true))

	_, events := do(t, h, "GET", "/api/events", "")
	results := events["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("logged %d events, want 2", len(results))
	}
	miss := results[0].(map[string]any)
	if miss["format"] != "vendor_match_hook" {
		t.Errorf("format = %v, want vendor_match_hook", miss["format"])
	}
	if errs := miss["errors"].([]any); len(errs) != 1 || errs[0] != "no_match" {
		t.Errorf("miss event errors = %v, want [no_match]", errs)
	}
}

func doCreateVendor(t *testing.T, h http.Handler, body string) {
	t.Helper()
	status, resp := do(t, h, "POST", "/api/vendors", body)
	if status != http.StatusCreated {
		t.Fatalf("create vendor failed: %d %v", status, resp)
	}
}
