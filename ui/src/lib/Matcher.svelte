<script>
  import { api } from './api.js'

  // A sandbox for GET /api/vendors/match: type what the document says and see
  // exactly what ERPX answers the caller.
  let form = $state({ vat: '', name: '', iban: '', account: '', tax_number: '' })
  let result = $state(null)
  let error = $state('')
  let busy = $state(false)

  const samples = [
    { label: 'Invoice 1 — VAT and name agree', vat: 'CZ34560613', name: 'Supplier s.r.o.' },
    { label: 'Invoice 2 — punctuated VAT, other name', vat: 'CZ-34560613', name: 'Vendor a.s.' },
    { label: 'Invoice 3 — no supplier name printed', vat: 'CZ-34560613', name: '' },
    { label: 'Vendor not in ERPX', vat: 'GB123456789', name: 'Totally Unknown Trading' },
  ]

  function loadSample(sample) {
    form = { vat: sample.vat, name: sample.name, iban: '', account: '', tax_number: '' }
    run()
  }

  async function run(event) {
    event?.preventDefault()
    busy = true
    try {
      const params = Object.fromEntries(Object.entries(form).filter(([, v]) => v !== ''))
      result = await api.matchVendor(params)
      error = ''
    } catch (e) {
      error = e.message
      result = null
    } finally {
      busy = false
    }
  }
</script>

<p class="muted">
  Vendor lookup: ERPX normalises the identifiers it is given (punctuation, spacing, accents,
  legal form) and answers which vendors they belong to. It does not score or rank, and it does
  not decide whether a document may proceed — that stays in Rossum.
</p>

<form onsubmit={run}>
  <div class="form-grid">
    <label>Supplier VAT number<input bind:value={form.vat} placeholder="CZ-34560613" /></label>
    <label>Supplier name<input bind:value={form.name} placeholder="Supplier s.r.o." /></label>
    <label>IBAN<input bind:value={form.iban} /></label>
    <label>Bank account<input bind:value={form.account} /></label>
    <label>Tax number<input bind:value={form.tax_number} /></label>
  </div>
  <div class="row"><button type="submit" aria-busy={busy}>Look up</button></div>
</form>

<div class="row">
  <small class="muted">Samples:</small>
  {#each samples as sample (sample.label)}
    <button class="secondary outline" onclick={() => loadSample(sample)}>{sample.label}</button>
  {/each}
</div>

{#if error}<p><mark>{error}</mark></p>{/if}

{#if result}
  <article>
    {#if result.matches.length === 0}
      <div class="row">
        <span class="badge err">no match</span>
        <strong>Nothing in ERPX master data matches this query.</strong>
      </div>
      <p class="muted">
        <small>The vendor has to be created in ERPX before an invoice can be booked
        against it.</small>
      </p>
    {:else}
      <div class="row">
        <span class="badge ok">{result.matches.length} match{result.matches.length > 1 ? 'es' : ''}</span>
        {#if result.matches.length > 1}
          <strong>Ambiguous master data — more than one vendor holds these identifiers.</strong>
        {/if}
      </div>
      <table>
        <thead>
          <tr><th>Code</th><th>Name</th><th>VAT number</th><th>Matched on</th></tr>
        </thead>
        <tbody>
          {#each result.matches as match (match.code)}
            <tr>
              <td class="nowrap"><strong>{match.code}</strong></td>
              <td>{match.name}</td>
              <td class="nowrap">{match.vat_number}</td>
              <td class="nowrap">
                {#each match.matched_on as field}
                  <span class="badge">{field}</span>&nbsp;
                {/each}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}

    <details>
      <summary>API response</summary>
      <pre class="json">{JSON.stringify(result, null, 2)}</pre>
    </details>
  </article>
{/if}
