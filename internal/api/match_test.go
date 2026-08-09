package api

import (
	"encoding/json"
	"testing"

	"github.com/unra73d/rossum-erp/internal/model"
	"github.com/unra73d/rossum-erp/internal/seed"
)

func testVendors(t *testing.T) []model.Vendor {
	t.Helper()
	var vendors []model.Vendor
	if err := json.Unmarshal(seed.Vendors, &vendors); err != nil {
		t.Fatalf("seed vendors: %v", err)
	}
	return vendors
}

func codesOf(matches []Match) []string {
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m.Code
	}
	return out
}

func TestMatchVendor(t *testing.T) {
	vendors := testVendors(t)

	tests := []struct {
		name      string
		query     MatchQuery
		wantCodes []string
		wantOn    []string
	}{
		{
			name:      "vat and name both resolve to the same vendor",
			query:     MatchQuery{VAT: "CZ34560613", Name: "Supplier s.r.o."},
			wantCodes: []string{"9999"},
			wantOn:    []string{"vat_number", "name"},
		},
		{
			// Punctuation is normalised away on both sides, so ERPX resolves
			// the VAT whether or not the Rossum hook cleaned it up first.
			name:      "punctuated vat",
			query:     MatchQuery{VAT: "CZ-34560613"},
			wantCodes: []string{"9999"},
			wantOn:    []string{"vat_number"},
		},
		{
			// Sample invoice 2 prints a different company name over a VAT
			// number that is in the master data. ERPX reports what the VAT
			// resolves to and that the name did not; Rossum decides.
			name:      "known vat, unknown name",
			query:     MatchQuery{VAT: "CZ-34560613", Name: "Vendor a.s."},
			wantCodes: []string{"9999"},
			wantOn:    []string{"vat_number"},
		},
		{
			name:      "name only, legal form and case normalised",
			query:     MatchQuery{Name: "BROWAR HERGATZ GmbH"},
			wantCodes: []string{"2385"},
			wantOn:    []string{"name"},
		},
		{
			name:      "accents folded",
			query:     MatchQuery{Name: "Browar Krakow"},
			wantCodes: []string{"5843"},
			wantOn:    []string{"name"},
		},
		{
			name:      "iban with spaces",
			query:     MatchQuery{IBAN: "DE53 5001 0517 4531 4176 79"},
			wantCodes: []string{"5300"},
			wantOn:    []string{"iban"},
		},
		{
			name:      "tax number resolves through either column",
			query:     MatchQuery{TaxNum: "KN9463158956M"},
			wantCodes: []string{"5843"},
			wantOn:    []string{"tax_number_2"},
		},
		{
			name:      "unknown vendor resolves to nothing",
			query:     MatchQuery{Name: "Totally Unknown Trading", VAT: "GB123456789"},
			wantCodes: []string{},
		},
		{
			// A partial name is not a match: ERPX does lookups, not guesses.
			name:      "partial name",
			query:     MatchQuery{Name: "Browar"},
			wantCodes: []string{},
		},
		{
			// "N/A" in the master data must never resolve a document that has
			// no IBAN either.
			name:      "placeholder master data",
			query:     MatchQuery{IBAN: "N/A"},
			wantCodes: []string{},
		},
		{
			name:      "empty query",
			query:     MatchQuery{},
			wantCodes: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchVendor(vendors, tc.query)

			gotCodes := codesOf(got.Matches)
			if len(gotCodes) != len(tc.wantCodes) {
				t.Fatalf("matches = %v, want %v", gotCodes, tc.wantCodes)
			}
			for i, code := range tc.wantCodes {
				if gotCodes[i] != code {
					t.Fatalf("matches = %v, want %v", gotCodes, tc.wantCodes)
				}
			}
			if tc.wantOn != nil {
				if got := got.Matches[0].MatchedOn; !sameStrings(got, tc.wantOn) {
					t.Errorf("matched_on = %v, want %v", got, tc.wantOn)
				}
			}
		})
	}
}

func TestMatchVendorAmbiguousMasterData(t *testing.T) {
	// Two vendors sharing a VAT number is broken master data. ERPX reports
	// both rather than picking one - choosing is not its call.
	vendors := []model.Vendor{
		{Code: "1", Name: "Alpha", VATNumber: "CZ111"},
		{Code: "2", Name: "Beta", VATNumber: "CZ111"},
	}
	got := MatchVendor(vendors, MatchQuery{VAT: "CZ111"})
	if want := []string{"1", "2"}; !sameStrings(codesOf(got.Matches), want) {
		t.Errorf("matches = %v, want %v", codesOf(got.Matches), want)
	}
}

func TestMatchVendorEchoesQuery(t *testing.T) {
	// The echoed query makes a hook's logs self-explanatory: it shows what
	// ERPX was actually asked, not what the caller believed it sent.
	got := MatchVendor(testVendors(t), MatchQuery{VAT: "CZ34560613", Name: ""})
	if len(got.Query) != 1 || got.Query["vat"] != "CZ34560613" {
		t.Errorf("query = %v, want only vat", got.Query)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
