package api

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/unra73d/rossum-erp/internal/model"
)

// Issue is one ERPX complaint about a posted document. The codes are stable so
// the Rossum side can branch on them and raise the message on the right field.
type Issue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// alnumOnly is the character set ERPX accepts in a VAT number. This is the
// constraint the VAT normalisation hook exists to satisfy - keeping the check
// here means a misconfigured queue fails loudly instead of writing junk.
var alnumOnly = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// dateLayout is the only date format ERPX accepts; Rossum's normalized_value
// already emits it.
const dateLayout = "2006-01-02"

// Validate applies the ERPX posting rules, returning blocking errors and
// non-blocking warnings.
func Validate(inv model.Invoice, vendorExists func(string) bool, today time.Time) (errs []Issue, warns []Issue) {
	add := func(field, code, format string, args ...any) {
		errs = append(errs, Issue{Field: field, Code: code, Message: fmt.Sprintf(format, args...)})
	}
	warn := func(field, code, format string, args ...any) {
		warns = append(warns, Issue{Field: field, Code: code, Message: fmt.Sprintf(format, args...)})
	}

	if strings.TrimSpace(inv.VendorCode) == "" {
		add("vendor_code", "vendor_code_missing", "Vendor code is required; ERPX cannot book an invoice without a vendor.")
	} else if !vendorExists(inv.VendorCode) {
		add("vendor_code", "vendor_not_found", "Vendor code %q does not exist in ERPX master data.", inv.VendorCode)
	}

	if strings.TrimSpace(inv.Number) == "" {
		add("number", "invoice_number_missing", "Invoice number is required.")
	}

	issued, issuedOK := parseDate(inv.IssueDate)
	switch {
	case strings.TrimSpace(inv.IssueDate) == "":
		add("issue_date", "issue_date_missing", "Issue date is required.")
	case !issuedOK:
		add("issue_date", "date_format_invalid", "Issue date %q is not in YYYY-MM-DD format.", inv.IssueDate)
	}

	if strings.TrimSpace(inv.DueDate) != "" {
		due, ok := parseDate(inv.DueDate)
		switch {
		case !ok:
			add("due_date", "date_format_invalid", "Due date %q is not in YYYY-MM-DD format.", inv.DueDate)
		case due.Before(today):
			add("due_date", "due_date_in_past", "Due date %s is in the past (today is %s).", inv.DueDate, today.Format(dateLayout))
		case issuedOK && due.Before(issued):
			warn("due_date", "due_date_before_issue", "Due date %s precedes the issue date %s.", inv.DueDate, inv.IssueDate)
		}
	}

	for field, vat := range map[string]string{
		"supplier_vat_number": inv.SupplierVAT,
		"customer_vat_number": inv.CustomerVAT,
	} {
		if vat != "" && !alnumOnly.MatchString(vat) {
			add(field, "vat_not_alphanumeric",
				"VAT number %q contains characters ERPX cannot process; only letters and digits are accepted.", vat)
		}
	}

	if inv.NetTotal == 0 {
		add("net_total", "net_total_missing", "Net total amount is required.")
	}

	for idx, li := range inv.LineItems {
		prefix := fmt.Sprintf("line_items[%d]", idx)
		if strings.TrimSpace(li.Description) == "" {
			add(prefix+".description", "line_description_missing", "Line %d has no description.", idx+1)
		}
		if li.Quantity == 0 {
			add(prefix+".quantity", "line_quantity_missing", "Line %d has no quantity.", idx+1)
		}
		if li.AmountTotalBase == 0 {
			add(prefix+".amount_total_base", "line_net_total_missing",
				"Line %d has no net total; it should be computed as quantity x unit price.", idx+1)
			continue
		}
		if expected := round2(li.Quantity * li.UnitPriceBase); li.UnitPriceBase != 0 && !nearlyEqual(expected, li.AmountTotalBase, 0.02) {
			warn(prefix+".amount_total_base", "line_net_total_mismatch",
				"Line %d net total %.2f does not equal quantity x unit price (%.2f).", idx+1, li.AmountTotalBase, expected)
		}
	}

	if len(inv.LineItems) > 0 && inv.NetTotal != 0 {
		var sum float64
		for _, li := range inv.LineItems {
			sum += li.AmountTotalBase
		}
		if !nearlyEqual(round2(sum), inv.NetTotal, 0.05) {
			warn("net_total", "totals_mismatch",
				"Line item net totals sum to %.2f but the document net total is %.2f.", round2(sum), inv.NetTotal)
		}
	}
	return errs, warns
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse(dateLayout, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func nearlyEqual(a, b, tolerance float64) bool { return math.Abs(a-b) <= tolerance }

func round2(f float64) float64 { return math.Round(f*100) / 100 }
