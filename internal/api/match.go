package api

import (
	"sort"
	"strings"
	"unicode"

	"github.com/unra73d/rossum-erp/internal/model"
)

// Vendor lookup. ERPX answers one question - "which vendor does this
// identifier belong to?" - and nothing more. It does not score, rank, or decide
// whether a document may proceed: extraction confidence and the block/allow
// decision belong to Rossum, which has its own confidence signals. Returning a
// number here would look like a Rossum confidence and would not be one.

// MatchQuery carries whatever the document yielded. Every field is optional;
// sample invoice 3 has no supplier name printed at all.
type MatchQuery struct {
	VAT     string
	Name    string
	IBAN    string
	Account string
	TaxNum  string
}

// Match is a vendor found by lookup, plus the field that found it.
type Match struct {
	model.Vendor
	MatchedOn []string `json:"matched_on"`
}

// MatchResult is the lookup answer: the normalised query that was run, and
// every vendor it resolved to. Zero matches is a normal answer, not an error.
type MatchResult struct {
	Query   map[string]string `json:"query"`
	Matches []Match           `json:"matches"`
}

// MatchVendor resolves a query against vendor master data by exact match on
// normalised identifiers, plus the normalised company name. Several matches
// mean the master data is ambiguous; the caller decides what to do about it.
func MatchVendor(vendors []model.Vendor, q MatchQuery) MatchResult {
	result := MatchResult{Query: map[string]string{}, Matches: []Match{}}
	for key, value := range map[string]string{
		"vat": q.VAT, "name": q.Name, "iban": q.IBAN,
		"account": q.Account, "tax_number": q.TaxNum,
	} {
		if strings.TrimSpace(value) != "" {
			result.Query[key] = value
		}
	}
	if len(result.Query) == 0 {
		return result
	}

	for _, v := range vendors {
		if fields := matchedFields(v, q); len(fields) > 0 {
			result.Matches = append(result.Matches, Match{Vendor: v, MatchedOn: fields})
		}
	}
	sort.Slice(result.Matches, func(i, j int) bool {
		return result.Matches[i].Code < result.Matches[j].Code
	})
	return result
}

// matchedFields lists every field of this vendor the query resolved to.
func matchedFields(v model.Vendor, q MatchQuery) []string {
	var fields []string
	add := func(name string, candidate, query string) {
		if query == "" || !usable(candidate) {
			return
		}
		if alnum(candidate) == alnum(query) {
			fields = append(fields, name)
		}
	}

	add("vat_number", v.VATNumber, q.VAT)
	add("iban", v.IBAN, q.IBAN)
	add("bank_account", v.BankAccount, q.Account)
	add("tax_number_1", v.TaxNumber1, q.TaxNum)
	add("tax_number_2", v.TaxNumber2, q.TaxNum)

	if q.Name != "" && v.Name != "" && normalisedName(v.Name) == normalisedName(q.Name) {
		fields = append(fields, "name")
	}
	return fields
}

// usable rejects placeholder master data such as the literal "N/A", which
// would otherwise match a document that has no IBAN either.
func usable(s string) bool {
	s = strings.TrimSpace(strings.ToUpper(s))
	return s != "" && s != "N/A" && s != "-"
}

// alnum lowercases and drops every non-alphanumeric rune, so "CZ-34560613",
// "CZ 345 606 13" and "cz34560613" are one and the same identifier.
func alnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// legalFormSuffixes are dropped when normalising a company name. They carry no
// distinguishing information and the punctuation around them varies by
// document, so "Supplier s.r.o." and "Supplier" are the same vendor.
var legalFormSuffixes = map[string]bool{
	"sro": true, "as": true, "spolsro": true, "gmbh": true, "ag": true,
	"ltd": true, "limited": true, "llc": true, "inc": true, "bv": true,
	"nv": true, "sa": true, "spzoo": true, "kg": true, "ohg": true, "se": true,
}

// normalisedName reduces a company name to a comparable form: lowercase, no
// accents, no punctuation, no legal form. This is normalisation, not fuzzy
// matching - two names either reduce to the same string or they do not.
func normalisedName(s string) string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var kept, all []string
	for _, f := range fields {
		f = foldAccents(f)
		if legalFormSuffixes[f] {
			continue
		}
		all = append(all, f)
		// Single characters are the remains of dotted legal forms ("s.r.o.").
		if len([]rune(f)) > 1 {
			kept = append(kept, f)
		}
	}
	// A name made only of initials keeps them rather than becoming empty.
	if len(kept) == 0 {
		kept = all
	}
	return strings.Join(kept, "")
}

// foldAccents maps the accented characters common in CZ/DE/PL vendor names to
// ASCII so "Kraków" and "Krakow" resolve to the same vendor.
var accentFold = strings.NewReplacer(
	"á", "a", "ä", "a", "à", "a", "â", "a", "ą", "a",
	"č", "c", "ć", "c", "ç", "c",
	"ď", "d", "é", "e", "ě", "e", "ë", "e", "è", "e", "ę", "e",
	"í", "i", "ï", "i", "ł", "l", "ň", "n", "ń", "n",
	"ó", "o", "ö", "o", "ô", "o", "ř", "r",
	"š", "s", "ś", "s", "ß", "ss", "ť", "t",
	"ú", "u", "ů", "u", "ü", "u", "ý", "y",
	"ž", "z", "ź", "z", "ż", "z",
)

func foldAccents(s string) string { return accentFold.Replace(s) }
