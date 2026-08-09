<script>
  import { api, formatTime } from './api.js'

  // In production this list is replicated from the real ERP; here it is
  // editable so a demo can add or break a vendor and watch Rossum react.
  const blank = {
    code: '', name: '', address: '', postal_code: '', city: '', country: '',
    vat_number: '', iban: '', bank_account: '', tax_number_1: '', tax_number_2: '',
  }

  let vendors = $state([])
  let query = $state('')
  let error = $state('')
  let loading = $state(true)
  let editing = $state(null)
  let isNew = $state(false)

  async function load() {
    loading = true
    try {
      vendors = (await api.vendors(query)).results ?? []
      error = ''
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function startCreate() {
    editing = { ...blank }
    isNew = true
  }

  function startEdit(vendor) {
    editing = { ...vendor }
    isNew = false
  }

  async function save(event) {
    event.preventDefault()
    try {
      if (isNew) await api.createVendor(editing)
      else await api.updateVendor(editing.code, editing)
      editing = null
      await load()
    } catch (e) {
      error = e.message
    }
  }

  async function remove(vendor) {
    if (!confirm(`Delete vendor ${vendor.code} — ${vendor.name}?`)) return
    try {
      await api.deleteVendor(vendor.code)
      await load()
    } catch (e) {
      error = e.message
    }
  }

  load()
</script>

<div class="row spread">
  <input
    class="grow"
    type="search"
    placeholder="Search code, name, VAT, city…"
    bind:value={query}
    oninput={load} />
  <button onclick={startCreate}>Add vendor</button>
</div>

{#if error}<p><mark>{error}</mark></p>{/if}

{#if loading}
  <p aria-busy="true">Loading vendors…</p>
{:else if vendors.length === 0}
  <p class="empty">No vendors match.</p>
{:else}
  <figure>
    <table>
      <thead>
        <tr>
          <th>Code</th><th>Name</th><th>VAT number</th><th>Address</th>
          <th>IBAN / account</th><th>Updated</th><th></th>
        </tr>
      </thead>
      <tbody>
        {#each vendors as vendor (vendor.code)}
          <tr>
            <td class="nowrap"><strong>{vendor.code}</strong></td>
            <td>{vendor.name}</td>
            <td class="nowrap">{vendor.vat_number}</td>
            <td>
              {vendor.address}
              <br /><span class="muted">{vendor.postal_code} {vendor.city} {vendor.country}</span>
            </td>
            <td>
              {vendor.iban}
              {#if vendor.bank_account}<br /><span class="muted">{vendor.bank_account}</span>{/if}
            </td>
            <td class="nowrap muted">{formatTime(vendor.updated_at)}</td>
            <td class="actions nowrap">
              <button class="secondary outline" onclick={() => startEdit(vendor)}>Edit</button>
              <button class="secondary outline" onclick={() => remove(vendor)}>Delete</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </figure>
{/if}

{#if editing}
  <dialog open>
    <article>
      <header>
        <button aria-label="Close" rel="prev" onclick={() => (editing = null)}></button>
        <strong>{isNew ? 'New vendor' : `Vendor ${editing.code}`}</strong>
      </header>
      <form onsubmit={save}>
        <div class="form-grid">
          <label>
            Code
            <input bind:value={editing.code} required readonly={!isNew} />
          </label>
          <label>Name<input bind:value={editing.name} required /></label>
          <label>VAT number<input bind:value={editing.vat_number} /></label>
          <label>Address<input bind:value={editing.address} /></label>
          <label>Postal code<input bind:value={editing.postal_code} /></label>
          <label>City<input bind:value={editing.city} /></label>
          <label>Country<input bind:value={editing.country} /></label>
          <label>IBAN<input bind:value={editing.iban} /></label>
          <label>Bank account<input bind:value={editing.bank_account} /></label>
          <label>Tax number 1<input bind:value={editing.tax_number_1} /></label>
          <label>Tax number 2<input bind:value={editing.tax_number_2} /></label>
        </div>
        <footer class="row">
          <button type="submit">Save</button>
          <button type="button" class="secondary" onclick={() => (editing = null)}>Cancel</button>
        </footer>
      </form>
    </article>
  </dialog>
{/if}
