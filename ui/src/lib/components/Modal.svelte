<script>
  import Icon from './Icon.svelte'

  let { open = $bindable(false), title = '', subtitle = '', wide = false, onClose = null, children } = $props()

  let dialogEl

  $effect(() => {
    if (open && dialogEl && !dialogEl.open) dialogEl.showModal()
    if (!open && dialogEl?.open) dialogEl.close()
  })

  function close() {
    open = false
    onClose?.()
  }
</script>

<dialog
  bind:this={dialogEl}
  aria-labelledby="modal-title"
  onclose={() => { if (open) close() }}
  oncancel={(event) => { event.preventDefault(); close() }}
>
  <div class="dialog-box" class:dialog-wide={wide}>
    <div class="dialog-head">
      <div class="card-title-group">
        <h2 id="modal-title">{title}</h2>
        {#if subtitle}<p class="muted tiny">{subtitle}</p>{/if}
      </div>
      <button type="button" class="icon-button" aria-label="Cerrar ventana" onclick={close}>
        <Icon name="close" size={15} />
      </button>
    </div>
    <div class="dialog-body">
      {@render children?.()}
    </div>
  </div>
</dialog>
