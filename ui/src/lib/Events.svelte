<script>
  import { api, formatTime } from './api.js'

  let events = $state([])
  let error = $state('')
  let loading = $state(true)
  let auto = $state(false)
  let timer

  async function load() {
    try {
      events = (await api.events()).results ?? []
      error = ''
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  // Polling is opt-in so a projected demo screen can follow a live upload.
  $effect(() => {
    clearInterval(timer)
    if (auto) timer = setInterval(load, 3000)
    return () => clearInterval(timer)
  })

  async function clear() {
    if (!confirm('Clear the webhook log?')) return
    await api.clearEvents()
    await load()
  }

  function badge(status) {
    if (status < 300) return 'ok'
    if (status < 500) return 'warn'
    return 'err'
  }

  load()
</script>

<div class="row spread">
  <p class="grow muted">
    Every call to <code>POST /api/invoices</code>, accepted or refused — the first place to look
    when a document does not arrive.
  </p>
  <label class="nowrap"><input type="checkbox" role="switch" bind:checked={auto} /> Auto-refresh</label>
  <button class="secondary" onclick={load}>Refresh</button>
  <button class="secondary outline" onclick={clear}>Clear</button>
</div>

{#if error}<p><mark>{error}</mark></p>{/if}

{#if loading}
  <p aria-busy="true">Loading…</p>
{:else if events.length === 0}
  <p class="empty">No calls received yet.</p>
{:else}
  {#each events as event (event.id)}
    <article>
      <div class="row spread">
        <div class="row">
          <span class="badge {badge(event.status)}">{event.status}</span>
          <strong>{event.method} {event.path}</strong>
          {#if event.format}<span class="badge">{event.format}</span>{/if}
          {#if event.invoice_id}<span class="muted">→ {event.invoice_id}</span>{/if}
        </div>
        <span class="muted nowrap">{formatTime(event.at)}</span>
      </div>

      {#if event.errors?.length}
        <!-- Not keyed: one document can fail the same rule on two fields, so
             the same code legitimately appears twice. -->
        <p>
          {#each event.errors as code}
            <span class="badge err">{code}</span>&nbsp;
          {/each}
        </p>
      {/if}

      <details>
        <summary>Request body</summary>
        <pre class="json">{event.raw_request}</pre>
      </details>
    </article>
  {/each}
{/if}
