<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Browser, Events } from '@wailsio/runtime'
  import { NoteService } from '../bindings/github.com/dvher/pogo'
  import type { Note } from '../bindings/github.com/dvher/pogo/pkg/store/models'
  import { colorNames, colorOf } from './lib/colors'
  import { render, toggleTask } from './lib/markdown'

  let { id }: { id: string } = $props()

  let note = $state<Note | null>(null)
  let editing = $state(false)
  let draft = $state('')
  let showDots = $state(false)
  let showPalette = $state(false)
  let error = $state('')
  let textarea = $state<HTMLTextAreaElement>()

  let html = $derived(note ? render(note.content) : '')
  let color = $derived(colorOf(note?.color ?? 'yellow'))

  onMount(() => {
    NoteService.Get(id).then((n) => (note = n)).catch((e) => (error = String(e)))
    const off = Events.On('note:changed', (ev) => {
      const n = ev.data as Note
      if (n.id === id && !editing) note = n
    })
    return off
  })

  async function save(content: string) {
    try {
      note = await NoteService.SetContent(id, content)
      error = ''
    } catch (e) {
      error = String(e)
    }
  }

  function onClick(ev: MouseEvent) {
    if (editing) return
    const target = ev.target as HTMLElement
    const link = target.closest('a')
    if (link) {
      ev.preventDefault()
      Browser.OpenURL(link.href)
      return
    }
    const task = target.closest<HTMLElement>('li.task')
    if (task && note) {
      ev.preventDefault()
      save(toggleTask(note.content, Number(task.dataset.line)))
      return
    }
    if (target.closest('.dots, .palette')) return
    showDots = !showDots
    showPalette = false
  }

  async function startEdit() {
    if (!note) return
    draft = note.content
    editing = true
    showDots = showPalette = false
    await tick()
    textarea?.focus()
  }

  function finishEdit() {
    if (!editing) return
    editing = false
    if (note && draft !== note.content) save(draft)
  }

  function onEditKey(ev: KeyboardEvent) {
    if (ev.key === 'Escape' || (ev.key === 'Enter' && (ev.ctrlKey || ev.metaKey))) {
      ev.preventDefault()
      finishEdit()
    }
  }

  async function setColor(name: string) {
    showPalette = false
    note = await NoteService.SetColor(id, name)
  }

  async function toggleAnchor() {
    if (note) note = await NoteService.SetAnchored(id, !note.anchored)
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
  class="note"
  class:anchored={note?.anchored}
  class:editing
  style:--bg={color.bg}
  style:--ink={color.ink}
  style:--accent={color.accent}
  onclick={onClick}
>
  {#if note?.anchored}
    <svg class="pin-mark" viewBox="0 0 24 24" aria-label="Anchored"><path d="M16 3l5 5-3 1-4 4 1 5-2 2-4-4-5 5-1-1 5-5-4-4 2-2 5 1 4-4z" /></svg>
  {/if}

  {#if showDots && !editing}
    <div class="dots">
      <button class="dot" title="Color" onclick={() => (showPalette = !showPalette)}>
        <span class="swatch"></span>
      </button>
      <button class="dot" title="Edit (Markdown)" onclick={startEdit}>
        <svg viewBox="0 0 24 24"><path d="M4 20h4L19 9l-4-4L4 16zM14 6l4 4" /></svg>
      </button>
      <button class="dot" title="Hide note" onclick={() => NoteService.Hide(id)}>
        <svg viewBox="0 0 24 24"><path d="M3 3l18 18M10.6 5.1A10 10 0 0 1 12 5c5 0 9 4.5 10 7a13 13 0 0 1-3 4.2M6.6 6.6C4.2 8 2.6 10.3 2 12c1 2.5 5 7 10 7a10 10 0 0 0 4.4-1M9.9 9.9a3 3 0 0 0 4.2 4.2" /></svg>
      </button>
      <button class="dot" class:active={note?.anchored} title={note?.anchored ? 'Unanchor' : 'Anchor in place'} onclick={toggleAnchor}>
        <svg viewBox="0 0 24 24"><path d="M16 3l5 5-3 1-4 4 1 5-2 2-4-4-5 5-1-1 5-5-4-4 2-2 5 1 4-4z" /></svg>
      </button>
    </div>
    {#if showPalette}
      <div class="palette">
        {#each colorNames as name}
          <button
            class="swatch-btn"
            class:current={note?.color === name}
            title={name}
            aria-label={name}
            style:background={colorOf(name).bg}
            onclick={() => setColor(name)}
          ></button>
        {/each}
      </div>
    {/if}
  {/if}

  {#if editing}
    <textarea
      bind:this={textarea}
      bind:value={draft}
      onblur={finishEdit}
      onkeydown={onEditKey}
      spellcheck="false"
      placeholder="Write Markdown…  (Esc or Ctrl+Enter to finish)"
    ></textarea>
  {:else}
    <div class="content">{@html html}</div>
  {/if}

  {#if error}<div class="error">{error}</div>{/if}

  {#if !note?.anchored && !editing}
    <!-- Visual hint only: the Wails runtime resizes frameless windows natively
         from their edges and corners (disabled while anchored). -->
    <div class="grip"></div>
  {/if}
</div>

<style>
  .note {
    position: fixed;
    inset: 0;
    background: var(--bg);
    color: var(--ink);
    padding: 14px 16px 18px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    --wails-draggable: drag;
    cursor: default;
    box-shadow: inset 0 -18px 24px -20px rgb(0 0 0 / 0.18);
  }
  .note.anchored,
  .note.editing {
    --wails-draggable: no-drag;
  }

  .content {
    flex: 1;
    overflow-y: auto;
    font-size: 14px;
    line-height: 1.45;
    word-wrap: break-word;
    scrollbar-width: thin;
  }
  .content :global(h1),
  .content :global(h2),
  .content :global(h3) {
    margin: 0 0 6px;
    line-height: 1.2;
  }
  .content :global(h1) { font-size: 18px; }
  .content :global(h2) { font-size: 16px; }
  .content :global(h3) { font-size: 14px; }
  .content :global(p) { margin: 0 0 8px; }
  .content :global(ul),
  .content :global(ol) { margin: 0 0 8px; padding-left: 20px; }
  .content :global(li.task) {
    list-style: none;
    margin-left: -20px;
    cursor: pointer;
    border-radius: 4px;
    padding: 1px 2px;
  }
  .content :global(li.task:hover) { background: rgb(0 0 0 / 0.06); }
  .content :global(li.task.done) {
    text-decoration: line-through;
    text-decoration-thickness: 2px;
    opacity: 0.55;
  }
  .content :global(.task-box) {
    accent-color: var(--accent);
    margin: 0 6px 0 0;
    vertical-align: -2px;
    pointer-events: none;
  }
  .content :global(a) { color: inherit; text-decoration-color: var(--accent); }
  .content :global(code) {
    background: rgb(0 0 0 / 0.08);
    padding: 0 3px;
    border-radius: 3px;
    font-size: 12.5px;
  }
  .content :global(pre) {
    background: rgb(0 0 0 / 0.08);
    padding: 6px 8px;
    border-radius: 4px;
    overflow-x: auto;
  }
  .content :global(pre code) { background: none; padding: 0; }
  .content :global(blockquote) {
    margin: 0 0 8px;
    padding-left: 8px;
    border-left: 3px solid var(--accent);
  }
  .content :global(hr) { border: none; border-top: 1px solid var(--accent); }

  textarea {
    flex: 1;
    resize: none;
    border: none;
    outline: 1px dashed var(--accent);
    background: rgb(255 255 255 / 0.35);
    color: inherit;
    font: 13px/1.45 ui-monospace, 'JetBrains Mono', monospace;
    padding: 6px;
    border-radius: 4px;
  }

  .dots {
    position: absolute;
    top: 6px;
    right: 8px;
    display: flex;
    gap: 6px;
    z-index: 3;
    --wails-draggable: no-drag;
  }
  .dot {
    width: 22px;
    height: 22px;
    border-radius: 50%;
    border: 1px solid rgb(0 0 0 / 0.25);
    background: rgb(255 255 255 / 0.75);
    display: grid;
    place-items: center;
    padding: 0;
    cursor: pointer;
    color: var(--ink);
    box-shadow: 0 1px 3px rgb(0 0 0 / 0.2);
    animation: pop 0.12s ease-out;
  }
  .dot:hover { background: #fff; }
  .dot.active { background: var(--accent); color: #fff; }
  .dot svg {
    width: 13px;
    height: 13px;
    fill: none;
    stroke: currentColor;
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  .swatch {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: conic-gradient(#fff176, #f8bbd0, #d1c4e9, #b3e5fc, #c5e1a5, #ffcc80, #fff176);
  }
  @keyframes pop {
    from { transform: scale(0.4); opacity: 0; }
  }

  .palette {
    position: absolute;
    top: 34px;
    right: 8px;
    display: flex;
    gap: 5px;
    padding: 5px;
    background: rgb(255 255 255 / 0.9);
    border-radius: 14px;
    box-shadow: 0 2px 8px rgb(0 0 0 / 0.2);
    z-index: 3;
    --wails-draggable: no-drag;
  }
  .swatch-btn {
    width: 18px;
    height: 18px;
    border-radius: 50%;
    border: 1px solid rgb(0 0 0 / 0.25);
    padding: 0;
    cursor: pointer;
  }
  .swatch-btn.current { outline: 2px solid #333; outline-offset: 1px; }

  .pin-mark {
    position: absolute;
    top: 4px;
    left: 4px;
    width: 12px;
    height: 12px;
    fill: var(--accent);
    opacity: 0.8;
  }

  .grip {
    position: absolute;
    right: 0;
    bottom: 0;
    width: 16px;
    height: 16px;
    pointer-events: none;
    background: linear-gradient(135deg, transparent 55%, var(--accent) 55%, var(--accent) 62%, transparent 62%, transparent 75%, var(--accent) 75%, var(--accent) 82%, transparent 82%);
    opacity: 0;
    transition: opacity 0.15s;
  }
  .note:hover .grip { opacity: 0.8; }

  .error {
    font-size: 11px;
    color: #b00020;
    margin-top: 4px;
  }
</style>
