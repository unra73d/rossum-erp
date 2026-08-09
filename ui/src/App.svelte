<script>
  import Vendors from './lib/Vendors.svelte'
  import Invoices from './lib/Invoices.svelte'
  import Events from './lib/Events.svelte'
  import Matcher from './lib/Matcher.svelte'

  const tabs = [
    { id: 'invoices', label: 'Invoices', component: Invoices },
    { id: 'vendors', label: 'Vendors', component: Vendors },
    { id: 'matcher', label: 'Vendor matching', component: Matcher },
    { id: 'events', label: 'Webhook log', component: Events },
  ]

  // The tab lives in the hash so a reload (and a shared link) keeps its place.
  let active = $state(tabs.find((t) => t.id === location.hash.slice(1))?.id ?? 'invoices')
  const Current = $derived(tabs.find((t) => t.id === active).component)

  function select(id) {
    active = id
    location.hash = id
  }
</script>

<main class="container">
  <hgroup>
    <h1>ERPX</h1>
    <p>Vendor master data, invoice intake, and integration activity log.</p>
  </hgroup>

  <nav class="tabs">
    {#each tabs as tab (tab.id)}
      <button
        class:outline={active !== tab.id}
        class="secondary"
        onclick={() => select(tab.id)}>{tab.label}</button>
    {/each}
  </nav>

  <Current />
</main>
