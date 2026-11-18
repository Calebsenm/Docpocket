<script>
  let {
    kind = 'empty',
    title = '',
    detail = '',
    retryLabel = 'Reintentar',
    onRetry = null,
  } = $props()
</script>

<div class="state state-{kind}" class:state-inline={kind === 'inline'}>
  {#if kind === 'loading'}
    <span class="spinner spinner-lg" aria-hidden="true"></span>
    <span class="state-title">{title || 'Cargando…'}</span>
    {#if detail}<span class="state-detail">{detail}</span>{/if}
  {:else}
    <span class="state-icon" aria-hidden="true">{kind === 'error' ? '⚠' : kind === 'inline' ? 'ℹ' : '◇'}</span>
    <span class="state-title">{title || (kind === 'error' ? 'No se pudo cargar' : 'Sin datos')}</span>
    {#if detail}<span class="state-detail">{detail}</span>{/if}
    {#if onRetry}
      <button type="button" class="btn btn-sm" onclick={onRetry}>{retryLabel}</button>
    {/if}
  {/if}
</div>
