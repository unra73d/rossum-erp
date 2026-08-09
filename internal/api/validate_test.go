package api

import (
	"testing"
	"time"

	"github.com/unra73d/rossum-erp/internal/model"
)

func refDate(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
}

// validInvoice is a document ERPX accepts, used as the baseline each case
// breaks in exactly one way.
func validInvoice() model.Invoice {
	return model.Invoice{
		VendorCode:  "9999",
		Number:      "202201",
		IssueDate:   "2026-07-01",
		DueDate:     "2026-09-01",
		NetTotal:    247000,
		SupplierVAT: "CZ34560613",
		LineItems: []model.LineItem{
			{Description: "product 1", Quantity: 1, UnitPriceBase: 120000, AmountTotalBase: 120000},
			{Description: "product 2", Quantity: 2, UnitPriceBase: 63500, AmountTotalBase: 127000},
		},
	}
}

func codes(issues []Issue) []string {
	out := make([]string, len(issues))
	for i, is := range issues {
		out[i] = is.Code
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func knownVendor(code string) bool { return code == "9999" }

func TestValidateAcceptsCleanInvoice(t *testing.T) {
	errs, warns := Validate(validInvoice(), knownVendor, refDate(t))
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", codes(errs))
	}
	if len(warns) > 0 {
		t.Errorf("unexpected warnings: %v", codes(warns))
	}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*model.Invoice)
		want    string
		warning bool
	}{
		{
			name:   "vat with punctuation is what the VAT hook exists to prevent",
			mutate: func(i *model.Invoice) { i.SupplierVAT = "CZ-34560613" },
			want:   "vat_not_alphanumeric",
		},
		{
			name:   "vat with spaces",
			mutate: func(i *model.Invoice) { i.CustomerVAT = "CZ/123 64 765" },
			want:   "vat_not_alphanumeric",
		},
		{
			name:   "due date in the past",
			mutate: func(i *model.Invoice) { i.DueDate = "2021-12-31" },
			want:   "due_date_in_past",
		},
		{
			name:   "unknown vendor blocks the posting",
			mutate: func(i *model.Invoice) { i.VendorCode = "1234" },
			want:   "vendor_not_found",
		},
		{
			name:   "missing vendor code",
			mutate: func(i *model.Invoice) { i.VendorCode = "" },
			want:   "vendor_code_missing",
		},
		{
			name:   "line without the computed net total",
			mutate: func(i *model.Invoice) { i.LineItems[1].AmountTotalBase = 0 },
			want:   "line_net_total_missing",
		},
		{
			name:   "non-ISO date",
			mutate: func(i *model.Invoice) { i.IssueDate = "01/07/2026" },
			want:   "date_format_invalid",
		},
		{
			name:    "line net total that is not quantity x unit price",
			mutate:  func(i *model.Invoice) { i.LineItems[0].AmountTotalBase = 99 },
			want:    "line_net_total_mismatch",
			warning: true,
		},
		{
			name:    "line items that do not add up to the document total",
			mutate:  func(i *model.Invoice) { i.NetTotal = 1 },
			want:    "totals_mismatch",
			warning: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := validInvoice()
			tc.mutate(&inv)
			errs, warns := Validate(inv, knownVendor, refDate(t))

			got, label := codes(errs), "error"
			if tc.warning {
				got, label = codes(warns), "warning"
				if len(errs) > 0 {
					t.Errorf("expected only a warning, also got errors %v", codes(errs))
				}
			}
			if !contains(got, tc.want) {
				t.Errorf("missing %s %q, got %v", label, tc.want, got)
			}
		})
	}
}

func TestValidateDueDateToday(t *testing.T) {
	// Due today is still payable; only strictly past dates are an error.
	inv := validInvoice()
	inv.DueDate = "2026-08-09"
	if errs, _ := Validate(inv, knownVendor, refDate(t)); len(errs) > 0 {
		t.Errorf("due today rejected: %v", codes(errs))
	}
}
