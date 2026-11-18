<script>
  import { parseJson, lineNumbers } from '../format.js'

  let {
    value = $bindable(''),
    rows = 12,
    label = 'JSON',
    hint = '',
    readonly = false,
  } = $props()

  let parsed = $derived(parseJson(value))
  const lines = $derived(lineNumbers(value))

  function format() {
    if (parsed.ok) value = JSON.stringify(parsed.value, null, 2)
  }
</script>

<div class="field">
  <div class="field-label row">
    <span>{label}</span>
    <span class="spacer"></span>
    <button type="button" class="btn btn-xs btn-ghost" onclick={format} disabled={!parsed.ok || readonly}>
      Formatear JSON
    </button>
  </div>
  <div class="editor-shell">
    <div class="editor-toolbar">
      <span class="tiny muted">{readonly ? 'Solo lectura' : 'Edita el contenido JSON'}</span>
      <span class="badge" class:badge-danger={!parsed.ok} class:badge-ok={parsed.ok}>
        {parsed.ok ? 'JSON válido' : 'JSON inválido'}
      </span>
    </div>
    <div class="editor-main">
      <div class="editor-gutter" aria-hidden="true">{lines.join('\n')}</div>
      <textarea
        class="editor-area"
        class:code-input={true}
        {rows}
        {readonly}
        spellcheck="false"
        aria-label={label}
        bind:value
      ></textarea>
    </div>
  </div>
  {#if !parsed.ok}
    <p class="field-error">{parsed.error}</p>
  {:else if hint}
    <p class="field-hint">{hint}</p>
  {/if}
</div>
