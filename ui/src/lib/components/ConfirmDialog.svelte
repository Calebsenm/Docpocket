<script>
  import { notify } from '../stores.js'
  import Icon from './Icon.svelte'

  let {
    open = $bindable(false),
    title = 'Confirmar acción',
    message = '',
    confirmLabel = 'Confirmar',
    cancelLabel = 'Cancelar',
    danger = false,
    requireText = '',
    onConfirm = null,
    onCancel = null,
  } = $props()

  let dialogEl
  let typed = $state('')
  let busy = $state(false)
  let wasOpen = false

  const canConfirm = $derived(!busy && (!requireText || typed === requireText))

  $effect(() => {
    if (open && !wasOpen) {
      typed = ''
      busy = false
    }
    wasOpen = open
    if (open && dialogEl && !dialogEl.open) dialogEl.showModal()
    if (!open && dialogEl?.open) dialogEl.close()
  })

  function cancel() {
    if (busy) return
    open = false
    onCancel?.()
  }

  async function confirm() {
    if (!canConfirm) return
    busy = true
    try {
      await onConfirm?.()
      open = false
    } catch (error) {
      notify(error instanceof Error ? error.message : String(error), 'error')
    } finally {
      busy = false
    }
  }

  function handleClose() {
    if (open) cancel()
  }
</script>

<dialog
  bind:this={dialogEl}
  aria-labelledby="confirm-title"
  onclose={handleClose}
  oncancel={(event) => { event.preventDefault(); cancel() }}
>
  <div class="dialog-box">
    <div class="dialog-head">
      <h2 id="confirm-title">{title}</h2>
      <button type="button" class="icon-button" aria-label="Cerrar" onclick={cancel} disabled={busy}>
        <Icon name="close" size={15} />
      </button>
    </div>
    <div class="dialog-body stack">
      {#if message}<p class="wrap-text">{message}</p>{/if}
      {#if requireText}
        <div class="field">
          <label class="field-label" for="confirm-text">
            Escribe <strong class="mono">{requireText}</strong> para continuar
          </label>
          <input
            id="confirm-text"
            class="input mono"
            type="text"
            autocomplete="off"
            spellcheck="false"
            bind:value={typed}
            disabled={busy}
          />
        </div>
      {/if}
    </div>
    <div class="dialog-foot">
      <button type="button" class="btn" onclick={cancel} disabled={busy}>{cancelLabel}</button>
      <button
        type="button"
        class="btn {danger ? 'btn-danger' : 'btn-primary'}"
        onclick={confirm}
        disabled={!canConfirm}
      >
        {#if busy}<span class="spinner" aria-hidden="true"></span>{/if}
        {confirmLabel}
      </button>
    </div>
  </div>
</dialog>
