package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/unra73d/rossum-erp/internal/model"
	"github.com/unra73d/rossum-erp/internal/rossum"
)

// This file implements Rossum's webhook hook contract directly, so vendor
// matching can run as a webhook extension instead of a serverless function.
// The distinction matters: a serverless function is your code running inside
// Rossum's sandbox, which has no internet access by default. A webhook is
// Rossum's own infrastructure making an outbound call to a URL you host -
// the same category of thing as notifying Slack - so it is never subject to
// that restriction. Point a webhook extension at POST /rossum/vendor-match,
// triggered on annotation_content, and this responds with the same
// operations/messages shape a serverless function would return.

// rossumHookResponse is the body Rossum expects back from a content hook.
type rossumHookResponse struct {
	Operations []rossumOperation `json:"operations,omitempty"`
	Messages   []rossumMessage   `json:"messages,omitempty"`
}

// rossumOperation writes a value into one datapoint, addressed by its id
// (not schema_id).
type rossumOperation struct {
	Op    string             `json:"op"`
	ID    json.Number        `json:"id"`
	Value rossumFieldContent `json:"value"`
}

type rossumFieldContent struct {
	Content rossumValueBox `json:"content"`
}

type rossumValueBox struct {
	Value string `json:"value"`
}

// rossumMessage surfaces on the validation screen, optionally anchored to a
// field. An "error" message blocks confirmation; "warning" and "info" do not.
type rossumMessage struct {
	Type    string      `json:"type"`
	ID      json.Number `json:"id,omitempty"`
	Content string      `json:"content"`
}

func replaceOp(id json.Number, value string) rossumOperation {
	return rossumOperation{Op: "replace", ID: id, Value: rossumFieldContent{Content: rossumValueBox{Value: value}}}
}

// vendorMatchHook is the webhook target for vendor matching. It always
// answers 200 with a rossumHookResponse - even when the lookup fails or the
// payload is malformed - because a non-2xx or unparseable body would give
// the operator no visible feedback at all, whereas a message on the
// validation screen at least explains what went wrong.
func (s *Server) vendorMatchHook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusOK, rossumHookResponse{
			Messages: []rossumMessage{{Type: "error", Content: "Request body too large."}},
		})
		return
	}

	var payload struct {
		Annotation *struct {
			Content []rossum.Node `json:"content"`
		} `json:"annotation"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Annotation == nil {
		s.store.LogEvent(model.Event{Method: r.Method, Path: r.URL.Path, Format: "vendor_match_hook", Status: http.StatusOK, Errors: []string{"payload_invalid"}, RawRequest: string(body)})
		writeJSON(w, http.StatusOK, rossumHookResponse{
			Messages: []rossumMessage{{Type: "error", Content: "Invalid or empty annotation payload."}},
		})
		return
	}

	content := payload.Annotation.Content
	vendorCode := rossum.FindNode(content, "vendor_code")
	if vendorCode == nil {
		s.logHookEvent(r, body, "schema_field_missing")
		writeJSON(w, http.StatusOK, rossumHookResponse{
			Messages: []rossumMessage{{Type: "error", Content: "Schema field \"vendor_code\" is missing; add it before enabling this hook."}},
		})
		return
	}

	vatNode := rossum.FindNode(content, "sender_vat_id")
	nameNode := rossum.FindNode(content, "sender_name")
	vat := strings.TrimSpace(rossum.Value(vatNode))
	name := strings.TrimSpace(rossum.Value(nameNode))

	if vat == "" && name == "" {
		s.logHookEvent(r, body, "no_input")
		writeJSON(w, http.StatusOK, rossumHookResponse{
			Messages: []rossumMessage{{
				Type:    "error",
				ID:      firstID(vatNode, nameNode),
				Content: "No supplier VAT number or name extracted — cannot look up the vendor.",
			}},
		})
		return
	}

	result := MatchVendor(s.store.Vendors(), MatchQuery{VAT: vat, Name: name})

	var resp rossumHookResponse
	var eventErrs []string
	switch len(result.Matches) {
	case 1:
		vendor := result.Matches[0]
		resp.Operations = []rossumOperation{replaceOp(vendorCode.ID, vendor.Code)}
		resp.Messages = []rossumMessage{{
			Type:    "info",
			ID:      vendorCode.ID,
			Content: fmt.Sprintf("Vendor %s — %s (matched on %s).", vendor.Code, vendor.Name, strings.Join(vendor.MatchedOn, ", ")),
		}}
	case 0:
		resp.Operations = []rossumOperation{replaceOp(vendorCode.ID, "")}
		resp.Messages = []rossumMessage{{
			Type:    "error",
			ID:      firstID(vatNode, nameNode),
			Content: "Vendor not recognised in ERPX. Have the vendor created there before exporting this invoice.",
		}}
		eventErrs = []string{"no_match"}
	default:
		codes := make([]string, len(result.Matches))
		for i, m := range result.Matches {
			codes[i] = m.Code
		}
		resp.Operations = []rossumOperation{replaceOp(vendorCode.ID, "")}
		resp.Messages = []rossumMessage{{
			Type:    "error",
			ID:      vendorCode.ID,
			Content: fmt.Sprintf("Multiple vendors match this VAT/name in ERPX (%s) — master data needs fixing.", strings.Join(codes, ", ")),
		}}
		eventErrs = []string{"ambiguous_match"}
	}

	s.store.LogEvent(model.Event{
		Method: r.Method, Path: r.URL.Path, Format: "vendor_match_hook",
		Status: http.StatusOK, Errors: eventErrs, RawRequest: string(body),
	})
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) logHookEvent(r *http.Request, body []byte, code string) {
	s.store.LogEvent(model.Event{
		Method: r.Method, Path: r.URL.Path, Format: "vendor_match_hook",
		Status: http.StatusOK, Errors: []string{code}, RawRequest: string(body),
	})
}

// firstID returns the id of whichever node is non-nil, so a message can be
// anchored to VAT if present or fall back to name.
func firstID(nodes ...*rossum.Node) json.Number {
	for _, n := range nodes {
		if n != nil {
			return n.ID
		}
	}
	return ""
}
