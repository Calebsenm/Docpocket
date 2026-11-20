<script>
  import Settings from './Settings.svelte'
  import Admins from './Admins.svelte'
  import Sessions from './Sessions.svelte'
  import Logs from './Logs.svelte'
  import Maintenance from './Maintenance.svelte'

  const tabs = [
    ['general', 'General'],
    ['admins', 'Superusuarios'],
    ['sessions', 'Sesiones'],
    ['logs', 'Registro'],
    ['maintenance', 'Mantenimiento'],
  ]

  let tab = $state('general')
</script>

<div class="stack">
  <div class="page-head">
    <div class="page-title">
      <h1>Ajustes</h1>
      <p class="page-subtitle">Configuración del servidor, accesos y mantenimiento.</p>
    </div>
  </div>

  <div class="tabs" role="tablist" aria-label="Secciones de ajustes">
    {#each tabs as [id, label] (id)}
      <button
        type="button"
        role="tab"
        class="tab"
        class:active={tab === id}
        aria-selected={tab === id}
        onclick={() => (tab = id)}
      >{label}</button>
    {/each}
  </div>

  <div class="mt-3 hub-content">
    {#if tab === 'general'}<Settings />
    {:else if tab === 'admins'}<Admins />
    {:else if tab === 'sessions'}<Sessions />
    {:else if tab === 'logs'}<Logs />
    {:else if tab === 'maintenance'}<Maintenance />
    {/if}
  </div>
</div>

<style>
  /* Cada pantalla conserva sus acciones, pero el título ya lo pone la pestaña. */
  .hub-content :global(.page-head) { margin-bottom: 14px; }
  .hub-content :global(.page-head > div:first-child) { display: none; }
  .hub-content :global(.page-head:not(:has(> :nth-child(2)))) { display: none; }
</style>