<script>
  import { api, formatMoney, formatTime } from './api.js'

  let invoices = $state([])
  let selected = $state(null)
  let error = $state('')
  let loading = $state(true)

  async function load() {
    loading = true
    try {
      invoices = (await api.invoices()).results ?? []
      error = ''
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  async function remove(invoice) {
    if (!confirm(`Delete ${invoice.id} (${invoice.number})?`)) return
    try {
      await api.deleteInvoice(invoice.id)
      if (selected?.id === invoice.id) selected = null
      await load()
    } catch (e) {
      error = e.message
    }
  }

  load()
</script>

<div class="row spread">
  <p class="grow muted">
    Invoices received via <code>POST /api/invoices</code>.
  </p>
  <button class="secondary" onclick={load}>Refresh</button>
</div>

{#if error}<p><mark>{error}</mark></p>{/if}

{#if loading}
  <p aria-busy="true">Loading invoices…</p>
{:else if invoices.length === 0}
  <p class="empty">
    Nothing posted yet. Send an invoice to <code>POST /api/invoices</code> to get started.
  </p>
{:else}
  <figure>
    <table>
      <thead>
        <tr>
          <th>ERPX id</th><th>Number</th><th>Vendor</th><th>Issued</th><th>Due</th>
          <th class="right">Net total</th><th>Lines</th><th>Received</th><th></th>
        </tr>
      </thead>
      <tbody>
        {#each invoices as invoice (invoice.id)}
          <tr>
            <td class="nowrap"><strong>{invoice.id}</strong></td>
            <td class="nowrap">{invoice.number}</td>
            <td class="nowrap">
              {invoice.vendor_code}
              <br /><span class="muted">{invoice.supplier_name ?? ''}</span>
            </td>
            <td class="nowrap">{invoice.issue_date}</td>
            <td class="nowrap">{invoice.due_date ?? ''}</td>
            <td class="right nowrap">{formatMoney(invoice.net_total, invoice.currency)}</td>
            <td class="right">{invoice.line_items?.length ?? 0}</td>
            <td class="nowrap muted">{formatTime(invoice.received_at)}</td>
            <td class="actions nowrap">
              <button class="secondary outline" onclick={() => (selected = invoice)}>View</button>
              <button class="secondary outline" onclick={() => remove(invoice)}>Delete</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </figure>
{/if}

{#if selected}
  <dialog open>
    <article>
      <header>
        <button aria-label="Close" rel="prev" onclick={() => (selected = null)}></button>
        <strong>{selected.id} — invoice {selected.number}</strong>
      </header>

      <div class="form-grid">
        <p><small class="muted">Vendor code</small><br />{selected.vendor_code}</p>
        <p><small class="muted">Supplier</small><br />{selected.supplier_name ?? '—'}</p>
        <p><small class="muted">Supplier VAT</small><br />{selected.supplier_vat_number ?? '—'}</p>
        <p><small class="muted">Customer</small><br />{selected.customer_name ?? '—'}</p>
        <p><small class="muted">Issue date</small><br />{selected.issue_date}</p>
        <p><small class="muted">Due date</small><br />{selected.due_date ?? '—'}</p>
        <p><small class="muted">Net / VAT / gross</small><br />
          {formatMoney(selected.net_total)} / {formatMoney(selected.vat_total)} /
          {formatMoney(selected.gross_total)} {selected.currency ?? ''}</p>
        <p><small class="muted">IBAN / account</small><br />
          {selected.iban ?? '—'} / {selected.account_number ?? '—'}</p>
        <p><small class="muted">Source document</small><br />{selected.document_name ?? '—'}</p>
        <p><small class="muted">Annotation</small><br />
          {#if selected.annotation_url}
            <a href={selected.annotation_url} target="_blank" rel="noreferrer">
              {selected.annotation_id}
            </a>
          {:else}—{/if}
        </p>
      </div>

      {#if selected.line_items?.length}
        <table>
          <thead>
            <tr>
              <th>#</th><th>Description</th><th class="right">Qty</th>
              <th class="right">Unit price</th><th class="right">VAT</th>
              <th class="right">Net total</th>
            </tr>
          </thead>
          <tbody>
            {#each selected.line_items as line, i (i)}
              <tr>
                <td>{i + 1}</td>
                <td>{line.description}</td>
                <td class="right">{line.quantity}</td>
                <td class="right">{formatMoney(line.unit_price_base)}</td>
                <td class="right">{line.vat_rate ?? ''}</td>
                <td class="right"><strong>{formatMoney(line.amount_total_base)}</strong></td>
              </tr>
            {/each}
          </tbody>
        </table>
        <p class="muted">
          <small>Net total is quantity × unit price.</small>
        </p>
      {/if}

      <details>
        <summary>Stored JSON</summary>
        <pre class="json">{JSON.stringify(selected, null, 2)}</pre>
      </details>
    </article>
  </dialog>
{/if}
