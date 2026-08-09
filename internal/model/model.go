// Package model holds the data structures ERPX exchanges with Rossum.
package model

import "time"

// Vendor is a master-data record. In a real deployment this table lives in the
// ERP and is replicated to Rossum; here it is the source of truth for the
// vendor matching step.
type Vendor struct {
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Address     string    `json:"address,omitempty"`
	PostalCode  string    `json:"postal_code,omitempty"`
	City        string    `json:"city,omitempty"`
	Country     string    `json:"country,omitempty"`
	VATNumber   string    `json:"vat_number,omitempty"`
	IBAN        string    `json:"iban,omitempty"`
	BankAccount string    `json:"bank_account,omitempty"`
	TaxNumber1  string    `json:"tax_number_1,omitempty"`
	TaxNumber2  string    `json:"tax_number_2,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// LineItem mirrors the Rossum line-item tuple. AmountTotalBase is the column
// CustomerX asked us to compute (quantity x unit price), since it is never
// printed on the document.
type LineItem struct {
	Code            string  `json:"code,omitempty"`
	Description     string  `json:"description"`
	Quantity        float64 `json:"quantity"`
	UnitPriceBase   float64 `json:"unit_price_base"`
	VATRate         float64 `json:"vat_rate,omitempty"`
	AmountTotalBase float64 `json:"amount_total_base"`
	AmountTotal     float64 `json:"amount_total,omitempty"`
}

// Invoice is the document as ERPX stores it.
type Invoice struct {
	ID         string `json:"id"`
	VendorCode string `json:"vendor_code"`
	Number     string `json:"number"`
	IssueDate  string `json:"issue_date"`
	DueDate    string `json:"due_date,omitempty"`
	Currency   string `json:"currency,omitempty"`

	NetTotal   float64 `json:"net_total"`
	VATTotal   float64 `json:"vat_total,omitempty"`
	GrossTotal float64 `json:"gross_total,omitempty"`

	SupplierName    string `json:"supplier_name,omitempty"`
	SupplierVAT     string `json:"supplier_vat_number,omitempty"`
	SupplierAddress string `json:"supplier_address,omitempty"`

	IBAN          string `json:"iban,omitempty"`
	SWIFT         string `json:"swift,omitempty"`
	AccountNumber string `json:"account_number,omitempty"`
	BankCode      string `json:"bank_code,omitempty"`

	CustomerName    string `json:"customer_name,omitempty"`
	CustomerVAT     string `json:"customer_vat_number,omitempty"`
	CustomerAddress string `json:"customer_address,omitempty"`

	LineItems []LineItem `json:"line_items,omitempty"`

	// Provenance, so a posted document can be traced back to Rossum.
	AnnotationID  string    `json:"annotation_id,omitempty"`
	AnnotationURL string    `json:"annotation_url,omitempty"`
	QueueURL      string    `json:"queue_url,omitempty"`
	DocumentName  string    `json:"document_name,omitempty"`
	ReceivedAt    time.Time `json:"received_at"`
}

// Event is one inbound webhook call, kept so the integration can be debugged
// from the UI without tailing server logs.
type Event struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     int       `json:"status"`
	InvoiceID  string    `json:"invoice_id,omitempty"`
	Format     string    `json:"format,omitempty"`
	Errors     []string  `json:"errors,omitempty"`
	RawRequest string    `json:"raw_request,omitempty"`
}
