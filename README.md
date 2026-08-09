# ERPX — mock ERP for the Rossum solution design

A stand-in for the CustomerX ERP, built so the Rossum configuration can be demonstrated
end to end: Rossum extracts and corrects the invoice, then posts it here, and the
document either lands in ERPX or is refused with a reason.

Two things it does that matter for the demo:

- **Accepts the invoice webhook.** `POST /api/invoices` takes either a flat JSON invoice
  or a raw Rossum annotation payload, and enforces the constraints the real ERPX has —
  alphanumeric VAT, a known vendor code, a due date that is not in the past, a net total
  on every line. Every call is logged and visible in the UI.
- **Serves the vendor master data.** `GET /api/vendors/match` resolves a VAT number, IBAN,
  bank account, tax number or company name to a vendor code. The vendor list is never
  copied into Rossum, so a vendor created in the ERP resolves immediately.

The scope stops there. Extraction, correction, confidence and the decision to block a
document all belong to Rossum; ERPX only serves master data, accepts postings, and refuses
what it cannot book. No authentication, by design: it is a demo target, not a system of
record.

## Run it

```bash
# everything in one binary (web/dist is already committed and embedded)
go run ./cmd/server                      # http://localhost:8080

# after changing ui/src, rebuild the embedded bundle
npm --prefix ui install && npm --prefix ui run build

# UI development with hot reload, API proxied to :8080
npm --prefix ui run dev                  # http://localhost:5173

# or a local container build (not what Render uses - see Deployment)
docker build -t erpx . && docker run -p 8080:8080 erpx
```

Try it against the sample payloads:

```bash
curl -X POST localhost:8080/api/invoices -H 'Content-Type: application/json' \
     -d @samples/rossum_webhook_ok.json         # 201, lands in ERPX

curl -X POST localhost:8080/api/invoices -H 'Content-Type: application/json' \
     -d @samples/rossum_webhook_rejected.json   # 422, one error per broken rule

curl 'localhost:8080/api/vendors/match?vat=CZ-34560613&name=Vendor%20a.s.'
```

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/vendors?q=` | Vendor list, searchable by code, name, VAT, city |
| `POST` `PUT` `DELETE` | `/api/vendors[/{code}]` | Vendor master data maintenance |
| `GET` | `/api/vendors/match` | Vendor lookup (pull) — `vat`, `name`, `iban`, `account`, `tax_number` |
| `POST` | `/rossum/vendor-match` | Vendor lookup as a Rossum webhook (push) — see below |
| `GET` | `/api/invoices` | Posted documents |
| `POST` | `/api/invoices` | **The invoice webhook target.** Flat or Rossum payload |
| `PUT` `DELETE` | `/api/invoices/{id}` | Correct or remove a posted document |
| `GET` `DELETE` | `/api/events` | Inbound call log — includes both vendor-match endpoints |
| `GET` | `/api/health` | Health check |

`POST /api/invoices` answers `201` (posted), `409` (this vendor and invoice number are
already booked — Rossum retries webhooks, and ERPX must not double-book), `422`
(validation failed, with one entry per problem), or `400` (unparseable body).

`GET /api/vendors/match` always answers `200`, with the normalised query it ran and every
vendor it resolved to. An empty `matches` array is a normal answer, so the caller reads the
body rather than handling a transport error:

```json
{
  "query": { "vat": "CZ-34560613", "name": "Vendor a.s." },
  "matches": [
    { "code": "9999", "name": "Supplier s.r.o.", "vat_number": "CZ34560613",
      "matched_on": ["vat_number"] }
  ]
}
```

Lookup is exact, after normalisation: punctuation and spacing are stripped from
identifiers (`CZ-34560613` and `cz 345 606 13` are the same VAT number), and names are
compared with accents folded and the legal form removed (`Browar Kraków` = `Browar
Krakow`, `Supplier s.r.o.` = `Supplier`). There is no scoring and no threshold — that is
deliberate. Rossum has its own confidence signals, and a number invented here would look
like one of them without being one.

`matched_on` is what makes the answer actionable. The example above is sample invoice 2:
the VAT number resolves to vendor 9999, the printed company name does not match it, and
ERPX reports exactly that. Whether a partial agreement is good enough to book is Rossum's
call, not the ERP's. Several matches means the master data itself is ambiguous.

### Two ways to wire vendor matching into Rossum

Both hit the same `MatchVendor` logic; they differ in who initiates the call, which
matters because of a Rossum platform restriction:

- **`GET /api/vendors/match`** — for a **serverless function**. Rossum's serverless
  functions have no internet access by default, except to the Rossum API itself; calling
  this endpoint from one requires Rossum granting an explicit exception for the account.
- **`POST /rossum/vendor-match`** — for a **webhook extension**. A webhook is Rossum's own
  infrastructure making an outbound call to a URL you host, the same category of thing as
  notifying Slack — not sandboxed user code, so it isn't subject to that restriction. Point
  a webhook extension at this path, triggered on `annotation_content`, and it speaks
  Rossum's own hook contract directly: it reads `sender_vat_id_sanitized`/`sender_name` out of the
  posted annotation tree and returns `operations`/`messages` in the shape Rossum expects,
  writing `vendor_code` (or a blocking error) with no function code needed on the Rossum
  side at all - see `internal/api/rossum_hook.go`.

Prefer the webhook route unless Rossum has already granted internet access for the queue.

## What Rossum has to be configured to send

Everything below is configured in Rossum; this repository does not contain any of it.

- **Two schema fields.** `vendor_code` on the header and `item_total_base` inside the
  `line_item` tuple, both `can_export: true`. ERPX rejects documents without them.
- **A vendor code that resolves.** Either wire a webhook extension at
  `POST /rossum/vendor-match` (recommended - see above), or a serverless function calling
  `GET /api/vendors/match` if internet access has been granted for the queue.
- **An export webhook** pointing at `POST /api/invoices`. No payload transformation is
  needed: the annotation content tree is flattened here, in `internal/rossum/parse.go`.

That mapping is a single table in that file (`headerFields` / `lineFields`), so a renamed
or added field is a one-line change. It always keys on `schema_id` — never on a label or a
column position, both of which move.

## What ERPX refuses, and why that is useful

| Rejection | Code | What it demonstrates |
| --- | --- | --- |
| VAT with punctuation | `vat_not_alphanumeric` | Why VAT normalisation has to happen in Rossum |
| Unknown or missing vendor code | `vendor_not_found`, `vendor_code_missing` | Why vendor matching has to happen before export |
| Due date before today | `due_date_in_past` | The rule ERPX will not bend |
| Line with no net total | `line_net_total_missing` | The computed column really is required |
| Same vendor and invoice number twice | `duplicate_invoice` (409) | Webhook retries must not double-book |

Rejections are the point, not a failure mode: they are what makes the *before* half of a
demo visible. ERPX never silently repairs a document — a mock ERP that patches over bad
data is how a misconfigured queue stays broken for a month.

## Layout

```
cmd/server/         entry point
internal/model/     the data ERPX exchanges with Rossum
internal/store/     mutex-guarded state, snapshotted to JSON
internal/rossum/    annotation tree -> flat invoice
internal/api/       HTTP handlers, validation rules, vendor lookup
internal/seed/      initial vendor list (from the supplied spreadsheet)
samples/            example webhook payloads
ui/                 Svelte 5 + Pico, built into web/dist and embedded
```

`go test ./...` covers the payload flattening, the matching thresholds, every validation
rule, and the HTTP surface.

## Deployment

`render.yaml` deploys one native Go web service — `go build -o bin/erpx ./cmd/server`,
nothing else. That only works because `web/dist` (the built UI) is committed to the repo,
so Render's build never touches Node. **After any change under `ui/src`, rebuild and
commit the bundle before pushing:**

```bash
npm --prefix ui run build
git add web/dist && git commit -m "Rebuild UI"
```

A stale `web/dist` is a silent trap - the server embeds whatever is on disk at compile
time, so an unrebuilt bundle deploys the *previous* UI with no build error to catch it.

A `Dockerfile` still exists for local container testing (`docker build -t erpx .`) but
Render does not use it - that multi-stage npm+go build is what made deploys slow.

On the free plan the filesystem is ephemeral and the instance sleeps when idle (first
request after can take 30-50s - a Render free-plan property, unrelated to the build), so
posted invoices are lost on restart while the vendor list re-seeds itself from
`internal/seed/vendors.json`. For anything longer-lived, switch to a paid plan and
uncomment the disk block in `render.yaml`.
