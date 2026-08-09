package rossum

import "testing"

// webhookBody mirrors the shape Rossum posts on annotation_content /
// annotation_status, including a nested line items multivalue.
const webhookBody = `{
  "action": "changed",
  "event": "annotation_status",
  "annotation": {
    "id": 3245786,
    "url": "https://api.elis.rossum.ai/v1/annotations/3245786",
    "queue": "https://api.elis.rossum.ai/v1/queues/8236",
    "status": "exporting",
    "content": [
      {"schema_id": "basic_info_section", "category": "section", "children": [
        {"schema_id": "document_id", "category": "datapoint", "content": {"value": "202201", "normalized_value": null}},
        {"schema_id": "date_issue", "category": "datapoint", "content": {"value": "31/12/2021", "normalized_value": "2021-12-31"}},
        {"schema_id": "date_due", "category": "datapoint", "content": {"value": "2/3/2099", "normalized_value": "2099-03-02"}},
        {"schema_id": "vendor_code", "category": "datapoint", "content": {"value": "9999"}}
      ]},
      {"schema_id": "vendor_section", "category": "section", "children": [
        {"schema_id": "sender_name", "category": "datapoint", "content": {"value": "Supplier s.r.o."}},
        {"schema_id": "sender_vat_id", "category": "datapoint", "content": {"value": "CZ34560613"}},
        {"schema_id": "account_num", "category": "datapoint", "content": {"value": "43-3242342345/0100"}}
      ]},
      {"schema_id": "amounts_section", "category": "section", "children": [
        {"schema_id": "amount_total_base", "category": "datapoint", "content": {"value": "897 000,00", "normalized_value": "897000"}},
        {"schema_id": "currency", "category": "datapoint", "content": {"value": "czk"}}
      ]},
      {"schema_id": "line_items_section", "category": "section", "children": [
        {"schema_id": "line_items", "category": "multivalue", "children": [
          {"schema_id": "line_item", "category": "tuple", "children": [
            {"schema_id": "item_description", "category": "datapoint", "content": {"value": "product 1"}},
            {"schema_id": "item_quantity", "category": "datapoint", "content": {"value": "1"}},
            {"schema_id": "item_amount_base", "category": "datapoint", "content": {"value": "120 000,00", "normalized_value": "120000"}},
            {"schema_id": "item_amount_total_base", "category": "datapoint", "content": {"value": "120000"}}
          ]},
          {"schema_id": "line_item", "category": "tuple", "children": [
            {"schema_id": "item_description", "category": "datapoint", "content": {"value": "product 2"}},
            {"schema_id": "item_quantity", "category": "datapoint", "content": {"value": "2"}},
            {"schema_id": "item_amount_base", "category": "datapoint", "content": {"value": "63500"}}
          ]},
          {"schema_id": "line_item", "category": "tuple", "children": [
            {"schema_id": "item_description", "category": "datapoint", "content": {"value": "", "normalized_value": null}}
          ]}
        ]}
      ]}
    ]
  },
  "document": {"file_name": "invoice_1.pdf", "mime_type": "application/pdf"}
}`

func TestParseWebhook(t *testing.T) {
	if !Looks([]byte(webhookBody)) {
		t.Fatal("Looks() did not recognise a Rossum webhook body")
	}
	inv, err := Parse([]byte(webhookBody))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if inv.Number != "202201" {
		t.Errorf("Number = %q, want 202201", inv.Number)
	}
	// The normalized value must win over the raw OCR text for dates.
	if inv.IssueDate != "2021-12-31" {
		t.Errorf("IssueDate = %q, want 2021-12-31", inv.IssueDate)
	}
	if inv.VendorCode != "9999" {
		t.Errorf("VendorCode = %q, want 9999", inv.VendorCode)
	}
	if inv.Currency != "CZK" {
		t.Errorf("Currency = %q, want CZK", inv.Currency)
	}
	if inv.NetTotal != 897000 {
		t.Errorf("NetTotal = %v, want 897000", inv.NetTotal)
	}
	if inv.AnnotationID != "3245786" {
		t.Errorf("AnnotationID = %q, want 3245786", inv.AnnotationID)
	}
	if inv.DocumentName != "invoice_1.pdf" {
		t.Errorf("DocumentName = %q, want invoice_1.pdf", inv.DocumentName)
	}

	// The empty placeholder tuple must not become a line item.
	if len(inv.LineItems) != 2 {
		t.Fatalf("got %d line items, want 2", len(inv.LineItems))
	}
	if got := inv.LineItems[0].AmountTotalBase; got != 120000 {
		t.Errorf("line 1 net total = %v, want 120000", got)
	}
	// Line 2 has no computed column. The parser leaves it at zero so
	// validation can reject it, instead of inventing the amount here.
	if got := inv.LineItems[1].AmountTotalBase; got != 0 {
		t.Errorf("line 2 net total = %v, want 0 (no silent repair)", got)
	}
	if got := inv.LineItems[1].UnitPriceBase; got != 63500 {
		t.Errorf("line 2 unit price = %v, want 63500", got)
	}
}

// realSchemaFieldNames uses the exact schema_ids a live Rossum tenant sent
// (item_total_base, amount_due), not the guessed names used in webhookBody
// above. A hand-written test fixture that reuses the same wrong guess as the
// parser it's testing proves nothing - that's exactly how item_total_base
// went unmapped: webhookBody used "item_amount_total_base" throughout, the
// parser recognised it, the test passed, and the mismatch only surfaced
// against a real export, which rejected every line with
// line_net_total_missing. This fixture exists so that class of bug fails
// here instead of live.
const realSchemaFieldNames = `{
  "action": "export", "event": "annotation_content",
  "annotation": {"id": 53567078, "content": [
    {"schema_id": "basic_info_section", "category": "section", "children": [
      {"schema_id": "document_id", "category": "datapoint", "content": {"value": "202201"}},
      {"schema_id": "date_issue", "category": "datapoint", "content": {"value": "10/31/2021", "normalized_value": "2021-10-31"}},
      {"schema_id": "date_due", "category": "datapoint", "content": {"value": "12/31/2026", "normalized_value": "2026-12-31"}}
    ]},
    {"schema_id": "totals_section", "category": "section", "children": [
      {"schema_id": "amount_total_base", "category": "datapoint", "content": {"value": "897,000.0", "normalized_value": "897000.0"}},
      {"schema_id": "amount_due", "category": "datapoint", "content": {"value": "1,085,370.0", "normalized_value": "1085370.0"}},
      {"schema_id": "currency", "category": "datapoint", "content": {"value": "czk"}}
    ]},
    {"schema_id": "vendor_section", "category": "section", "children": [
      {"schema_id": "sender_name", "category": "datapoint", "content": {"value": "Supplier s.r.o."}},
      {"schema_id": "sender_vat_id", "category": "datapoint", "content": {"value": "CZ34560613"}},
      {"schema_id": "vendor_code", "category": "datapoint", "content": {"value": "9999"}}
    ]},
    {"schema_id": "line_items_section", "category": "section", "children": [
      {"schema_id": "line_items", "category": "multivalue", "children": [
        {"schema_id": "line_item", "category": "tuple", "children": [
          {"schema_id": "item_description", "category": "datapoint", "content": {"value": "product 1"}},
          {"schema_id": "item_quantity", "category": "datapoint", "content": {"value": "1", "normalized_value": "1"}},
          {"schema_id": "item_amount_base", "category": "datapoint", "content": {"value": "120,000.00", "normalized_value": "120000.00"}},
          {"schema_id": "item_rate", "category": "datapoint", "content": {"value": "21", "normalized_value": "21"}},
          {"schema_id": "item_total_base", "category": "datapoint", "content": {"value": "120,000.0", "normalized_value": "120000.0"}},
          {"schema_id": "item_amount_total", "category": "datapoint", "content": {"value": "145,200.0", "normalized_value": "145200.0"}}
        ]}
      ]}
    ]}
  ]}
}`

func TestParseRealSchemaFieldNames(t *testing.T) {
	inv, err := Parse([]byte(realSchemaFieldNames))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if inv.GrossTotal != 1085370 {
		t.Errorf("GrossTotal (from amount_due) = %v, want 1085370", inv.GrossTotal)
	}
	if len(inv.LineItems) != 1 {
		t.Fatalf("got %d line items, want 1", len(inv.LineItems))
	}
	if got := inv.LineItems[0].AmountTotalBase; got != 120000 {
		t.Errorf("AmountTotalBase (from item_total_base) = %v, want 120000 - this is the field ERPX requires and rejects the line without", got)
	}
	if got := inv.LineItems[0].AmountTotal; got != 145200 {
		t.Errorf("AmountTotal (from item_amount_total) = %v, want 145200", got)
	}
}

func TestParseFlatIsNotRossum(t *testing.T) {
	if Looks([]byte(`{"number":"202201","vendor_code":"9999"}`)) {
		t.Error("a flat ERPX invoice was mistaken for a Rossum payload")
	}
}

func TestParseResultsEnvelope(t *testing.T) {
	body := `{"results":[{"id":7,"content":[{"schema_id":"basic_info_section","category":"section","children":[
		{"schema_id":"document_id","category":"datapoint","content":{"value":"A-1"}}]}]}]}`
	inv, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if inv.Number != "A-1" || inv.AnnotationID != "7" {
		t.Errorf("got number=%q id=%q, want A-1 / 7", inv.Number, inv.AnnotationID)
	}
}

func TestParseNumber(t *testing.T) {
	cases := map[string]float64{
		"":              0,
		"1234.56":       1234.56,
		"1 234,56":      1234.56,
		"1,234.56":      1234.56,
		"1.234,56":      1234.56,
		"897 000,00":    897000,
		"-42":           -42,
		"120 000,00 Kč": 120000,
		"n/a":           0,
	}
	for in, want := range cases {
		if got := ParseNumber(in); got != want {
			t.Errorf("ParseNumber(%q) = %v, want %v", in, got, want)
		}
	}
}
