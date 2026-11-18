<script>
  import { toasts, dismissToast } from '../stores.js'
  import Icon from './Icon.svelte'

  const symbols = { success: 'check', error: 'alert', info: 'activity' }
</script>

<div class="toast-stack" aria-live="polite">
  {#each $toasts as toast (toast.id)}
    <div
      class="toast toast-{toast.kind}"
      role={toast.kind === 'error' ? 'alert' : 'status'}
    >
      <Icon name={symbols[toast.kind]} size={15} />
      <span class="wrap-text">{toast.message}</span>
      <button type="button" aria-label="Cerrar aviso" onclick={() => dismissToast(toast.id)}>
        <Icon name="close" size={13} />
      </button>
    </div>
  {/each}
</div>
