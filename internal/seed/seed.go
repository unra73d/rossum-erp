// Package seed carries the initial vendor master data, standing in for the
// nightly vendor export a real ERP would push.
package seed

import _ "embed"

// Vendors is the JSON vendor list loaded when the store starts empty.
//
//go:embed vendors.json
var Vendors []byte
