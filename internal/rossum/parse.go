// Package rossum flattens a Rossum annotation content tree into the flat
// invoice ERPX understands.
//
// Rossum posts the annotation as a tree: sections contain datapoints, and the
// line items table is a multivalue of tuples. Every leaf carries a schema_id,
// which is the stable identifier to map against - never the label, never the
// position. That mapping lives in headerFields / lineFields below, so adapting
// to a renamed or added field is a one-line change.
package rossum

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/unra73d/rossum-erp/internal/model"
)

// Payload is the envelope Rossum sends to a webhook or export endpoint. Only
// the parts ERPX cares about are declared.
type Payload struct {
	Action     string `json:"action"`
	Event      string `json:"event"`
	Annotation *struct {
		ID      json.Number `json:"id"`
		URL     string      `json:"url"`
		Queue   string      `json:"queue"`
		Status  string      `json:"status"`
		Content []Node      `json:"content"`
	} `json:"annotation"`
	Document *struct {
		FileName string `json:"file_name"`
		MIMEType string `json:"mime_type"`
	} `json:"document"`
	// Export endpoints wrap annotations in results instead.
	Results []json.RawMessage `json:"results"`
}

// Node is one element of the annotation content tree. ID is the datapoint's
// database id - distinct from SchemaID - and is what a hook response must
// target when writing an operation back to this field.
type Node struct {
	ID       json.Number     `json:"id"`
	SchemaID string          `json:"schema_id"`
	Category string          `json:"category"`
	Value    json.RawMessage `json:"value"`
	Content  json.RawMessage `json:"content"`
	Children []Node          `json:"children"`
}

// nodeContent is the object form of Node.Content on a datapoint.
type nodeContent struct {
	Value           json.RawMessage `json:"value"`
	NormalizedValue json.RawMessage `json:"normalized_value"`
}

// headerFields maps Rossum schema ids to invoice header setters. Aliases exist
// because CustomerX may rename a field or add a custom one (vendor_code is
// custom - it is written by the vendor matching function).
var headerFields = map[string]func(*model.Invoice, string){
	"document_id":         func(i *model.Invoice, v string) { i.Number = v },
	"invoice_id":          func(i *model.Invoice, v string) { i.Number = v },
	"date_issue":          func(i *model.Invoice, v string) { i.IssueDate = v },
	"date_due":            func(i *model.Invoice, v string) { i.DueDate = v },
	"currency":            func(i *model.Invoice, v string) { i.Currency = strings.ToUpper(v) },
	"amount_total_base":   func(i *model.Invoice, v string) { i.NetTotal = ParseNumber(v) },
	"amount_total_tax":    func(i *model.Invoice, v string) { i.VATTotal = ParseNumber(v) },
	"amount_due":          func(i *model.Invoice, v string) { i.GrossTotal = ParseNumber(v) },
	"amount_total":        func(i *model.Invoice, v string) { i.GrossTotal = ParseNumber(v) },
	"sender_name":         func(i *model.Invoice, v string) { i.SupplierName = v },
	"supplier_name":       func(i *model.Invoice, v string) { i.SupplierName = v },
	"sender_vat_id":       func(i *model.Invoice, v string) { i.SupplierVAT = v },
	"supplier_vat_number": func(i *model.Invoice, v string) { i.SupplierVAT = v },
	"sender_address":      func(i *model.Invoice, v string) { i.SupplierAddress = v },
	"supplier_address":    func(i *model.Invoice, v string) { i.SupplierAddress = v },
	"recipient_name":      func(i *model.Invoice, v string) { i.CustomerName = v },
	"customer_name":       func(i *model.Invoice, v string) { i.CustomerName = v },
	"recipient_vat_id":    func(i *model.Invoice, v string) { i.CustomerVAT = v },
	"customer_vat_number": func(i *model.Invoice, v string) { i.CustomerVAT = v },
	"recipient_address":   func(i *model.Invoice, v string) { i.CustomerAddress = v },
	"customer_address":    func(i *model.Invoice, v string) { i.CustomerAddress = v },
	"iban":                func(i *model.Invoice, v string) { i.IBAN = v },
	"bic":                 func(i *model.Invoice, v string) { i.SWIFT = v },
	"swift":               func(i *model.Invoice, v string) { i.SWIFT = v },
	"account_num":         func(i *model.Invoice, v string) { i.AccountNumber = v },
	"bank_num":            func(i *model.Invoice, v string) { i.BankCode = v },
	"vendor_code":         func(i *model.Invoice, v string) { i.VendorCode = v },
	"supplier_code":       func(i *model.Invoice, v string) { i.VendorCode = v },
	"erp_vendor_code":     func(i *model.Invoice, v string) { i.VendorCode = v },
}

// lineFields maps schema ids inside a line_item tuple.
var lineFields = map[string]func(*model.LineItem, string){
	"item_code":              func(l *model.LineItem, v string) { l.Code = v },
	"item_id":                func(l *model.LineItem, v string) { l.Code = v },
	"item_description":       func(l *model.LineItem, v string) { l.Description = v },
	"item_quantity":          func(l *model.LineItem, v string) { l.Quantity = ParseNumber(v) },
	"item_amount_base":       func(l *model.LineItem, v string) { l.UnitPriceBase = ParseNumber(v) },
	"item_rate":              func(l *model.LineItem, v string) { l.VATRate = ParseNumber(v) },
	"item_tax_rate":          func(l *model.LineItem, v string) { l.VATRate = ParseNumber(v) },
	"item_total_base":        func(l *model.LineItem, v string) { l.AmountTotalBase = ParseNumber(v) },
	"item_amount_total_base": func(l *model.LineItem, v string) { l.AmountTotalBase = ParseNumber(v) },
	"item_net_total":         func(l *model.LineItem, v string) { l.AmountTotalBase = ParseNumber(v) },
	"item_amount_total":      func(l *model.LineItem, v string) { l.AmountTotal = ParseNumber(v) },
}

// Looks like a Rossum envelope rather than a flat ERPX invoice.
func Looks(body []byte) bool {
	var probe struct {
		Annotation json.RawMessage `json:"annotation"`
		Results    json.RawMessage `json:"results"`
		Content    json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	return probe.Annotation != nil || probe.Results != nil || probe.Content != nil
}

// Parse turns a Rossum webhook or export body into an invoice.
func Parse(body []byte) (model.Invoice, error) {
	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		return model.Invoice{}, fmt.Errorf("invalid Rossum payload: %w", err)
	}

	// Export endpoints deliver {"results": [annotation, ...]}; take the first.
	if p.Annotation == nil && len(p.Results) > 0 {
		wrapped := append(append([]byte(`{"annotation":`), p.Results[0]...), '}')
		if err := json.Unmarshal(wrapped, &p); err != nil {
			return model.Invoice{}, fmt.Errorf("invalid Rossum results payload: %w", err)
		}
	}
	// A bare annotation object ({"content": [...]}) is also accepted.
	if p.Annotation == nil {
		var bare struct {
			Content []Node `json:"content"`
		}
		if err := json.Unmarshal(body, &bare); err != nil || len(bare.Content) == 0 {
			return model.Invoice{}, fmt.Errorf("payload has no annotation content")
		}
		inv := model.Invoice{}
		walk(bare.Content, &inv)
		return inv, nil
	}

	inv := model.Invoice{
		AnnotationID:  p.Annotation.ID.String(),
		AnnotationURL: p.Annotation.URL,
		QueueURL:      p.Annotation.Queue,
	}
	if p.Document != nil {
		inv.DocumentName = p.Document.FileName
	}
	walk(p.Annotation.Content, &inv)
	return inv, nil
}

// walk descends the content tree, applying header setters to datapoints and
// collecting each line_item tuple.
func walk(nodes []Node, inv *model.Invoice) {
	for _, n := range nodes {
		switch n.Category {
		case "datapoint":
			if set, ok := headerFields[n.SchemaID]; ok {
				set(inv, value(n))
			}
		case "tuple":
			if item, ok := lineItem(n); ok {
				inv.LineItems = append(inv.LineItems, item)
			}
		default:
			walk(n.Children, inv)
		}
	}
}

// lineItem builds a line item from a tuple node, reporting whether it holds
// any recognised data (Rossum keeps empty placeholder rows).
func lineItem(n Node) (model.LineItem, bool) {
	var item model.LineItem
	found := false
	for _, child := range n.Children {
		if child.Category != "datapoint" {
			continue
		}
		set, ok := lineFields[child.SchemaID]
		if !ok {
			continue
		}
		v := value(child)
		if v == "" {
			continue
		}
		set(&item, v)
		found = true
	}
	// Deliberately no fallback for the computed net total: if the Rossum
	// formula field did not arrive, ERPX rejects the line rather than quietly
	// inventing an amount. Silent repair in the ERP is how a broken queue stays
	// broken for a month.
	return item, found
}

// FindNode returns the first node with this schema_id, searching depth-first
// through sections and multivalue/tuple children. Unlike Parse, which builds
// a whole invoice, this is for hooks that need a single datapoint's id and
// value - such as one reading sender_vat_id to look up a vendor.
func FindNode(nodes []Node, schemaID string) *Node {
	for i := range nodes {
		if nodes[i].SchemaID == schemaID {
			return &nodes[i]
		}
		if found := FindNode(nodes[i].Children, schemaID); found != nil {
			return found
		}
	}
	return nil
}

// Value returns a node's usable string value, or "" for a nil node - the
// exported counterpart to value(), for callers outside this package.
func Value(n *Node) string {
	if n == nil {
		return ""
	}
	return value(*n)
}

// value pulls the usable string out of a datapoint, preferring the normalized
// form (ISO dates, canonical numbers) over the raw OCR text.
func value(n Node) string {
	if len(n.Content) > 0 {
		var c nodeContent
		if err := json.Unmarshal(n.Content, &c); err == nil {
			if v := scalar(c.NormalizedValue); v != "" {
				return v
			}
			if v := scalar(c.Value); v != "" {
				return v
			}
		} else if v := scalar(n.Content); v != "" {
			return v
		}
	}
	return scalar(n.Value)
}

// scalar renders a JSON scalar as a trimmed string; anything else yields "".
func scalar(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var num json.Number
	if err := json.Unmarshal(raw, &num); err == nil {
		return num.String()
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return strconv.FormatBool(b)
	}
	return ""
}

// ParseNumber reads a number written in any of the formats an invoice may use:
// "1 234,56", "1,234.56", "1234.56", or with a trailing currency symbol.
func ParseNumber(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r == '.', r == ',', r == '-':
			b.WriteRune(r)
		}
	}
	s = b.String()
	lastComma := strings.LastIndex(s, ",")
	lastDot := strings.LastIndex(s, ".")
	switch {
	case lastComma > lastDot:
		// Comma is the decimal separator: drop dots, swap the comma.
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
	default:
		// Dot decimal (or none): commas are thousands separators.
		s = strings.ReplaceAll(s, ",", "")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
