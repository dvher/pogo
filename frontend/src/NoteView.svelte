<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Browser, Events } from '@wailsio/runtime'
  import { NoteService } from '../bindings/github.com/dvher/pogo'
  import type { Note } from '../bindings/github.com/dvher/pogo/pkg/store/models'
  import { colorNames, colorOf, hslHex, isCustom, toHSL } from './lib/colors'
  import { render, toggleTask } from './lib/markdown'

  let { id }: { id: string } = $props()

  let note = $state<Note | null>(null)
  let editing = $state(false)
  let draft = $state('')
  let showDots = $state(false)
  let showPalette = $state(false)
  let showPicker = $state(false)
  // Custom color being picked, as HSV so the square maps to saturation and
  // value; previewed live and saved when the pointer is released.
  let hsv = $state({ h: 0, s: 0, v: 1 })
  let preview = $state<string | null>(null)
  let hexInput = $state('')
  let error = $state('')
  let textarea = $state<HTMLTextAreaElement>()

  let html = $derived(note ? render(note.content) : '')
  let color = $derived(colorOf(preview ?? note?.color ?? 'yellow'))

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
    if (target.closest('.dots, .palette, .picker')) return
    showDots = !showDots
    showPalette = showPicker = false
    preview = null
  }

  async function startEdit() {
    if (!note) return
    draft = note.content
    editing = true
    showDots = showPalette = showPicker = false
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
    showPalette = showPicker = false
    preview = null
    note = await NoteService.SetColor(id, name)
  }

  function hsvHex({ h, s, v }: typeof hsv) {
    const l = v * (1 - s / 2)
    const sl = l === 0 || l === 1 ? 0 : (v - l) / Math.min(l, 1 - l)
    return hslHex(h, sl, l)
  }

  function openPicker() {
    showPicker = !showPicker
    if (!showPicker) return
    const bg = colorOf(note?.color ?? 'yellow').bg
    const [h, sl, l] = toHSL([1, 3, 5].map((i) => parseInt(bg.slice(i, i + 2), 16)) as [number, number, number])
    const v = l + sl * Math.min(l, 1 - l)
    hsv = { h, s: v === 0 ? 0 : 2 * (1 - l / v), v }
    hexInput = bg
  }

  function pick(next: typeof hsv) {
    hsv = next
    preview = hexInput = hsvHex(next)
  }

  function onSquare(ev: PointerEvent) {
    if (ev.type === 'pointerdown') (ev.currentTarget as HTMLElement).setPointerCapture(ev.pointerId)
    else if (!(ev.currentTarget as HTMLElement).hasPointerCapture(ev.pointerId)) return
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect()
    const clamp = (x: number) => Math.min(1, Math.max(0, x))
    pick({ h: hsv.h, s: clamp((ev.clientX - r.left) / r.width), v: clamp(1 - (ev.clientY - r.top) / r.height) })
  }

  // Saves the previewed color but keeps the picker open for more tweaks.
  async function commitPreview() {
    const picked = preview
    if (picked && picked !== note?.color) note = await NoteService.SetColor(id, picked)
    if (preview === picked) preview = null
  }

  function onHex(ev: Event) {
    const v = (ev.currentTarget as HTMLInputElement).value.trim().toLowerCase()
    const hex = v.startsWith('#') ? v : '#' + v
    if (isCustom(hex)) setColor(hex)
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
      <button class="dot" title="Color" onclick={() => ((showPalette = !showPalette), (showPicker = false), (preview = null))}>
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
        <button
          class="swatch-btn custom"
          class:current={note && isCustom(note.color)}
          title="Custom color"
          aria-label="Custom color"
          onclick={openPicker}
        ></button>
      </div>
      {#if showPicker}
        <div class="picker">
          <div
            class="sv"
            style:--hue={hsv.h}
            onpointerdown={onSquare}
            onpointermove={onSquare}
            onpointerup={commitPreview}
          >
            <span class="knob" style:left="{hsv.s * 100}%" style:top="{(1 - hsv.v) * 100}%"></span>
          </div>
          <input
            class="hue"
            type="range"
            min="0"
            max="359"
            value={Math.round(hsv.h)}
            oninput={(ev) => pick({ ...hsv, h: Number(ev.currentTarget.value) })}
            onchange={commitPreview}
            aria-label="Hue"
          />
          <input
            class="hex"
            value={hexInput}
            maxlength="7"
            spellcheck="false"
            aria-label="Hex color"
            onchange={onHex}
            onkeydown={(ev) => ev.key === 'Enter' && onHex(ev)}
          />
        </div>
      {/if}
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
  .swatch-btn.custom {
    background: conic-gradient(red, yellow, lime, cyan, blue, magenta, red);
  }

  .picker {
    position: absolute;
    top: 64px;
    right: 8px;
    width: 150px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 7px;
    background: rgb(255 255 255 / 0.95);
    border-radius: 10px;
    box-shadow: 0 2px 8px rgb(0 0 0 / 0.2);
    z-index: 3;
    --wails-draggable: no-drag;
  }
  .sv {
    position: relative;
    height: 90px;
    border-radius: 6px;
    cursor: crosshair;
    touch-action: none;
    background:
      linear-gradient(to top, #000, transparent),
      linear-gradient(to right, #fff, hsl(calc(var(--hue) * 1deg) 100% 50%));
  }
  .knob {
    position: absolute;
    width: 10px;
    height: 10px;
    margin: -6px 0 0 -6px;
    border: 2px solid #fff;
    border-radius: 50%;
    box-shadow: 0 0 0 1px rgb(0 0 0 / 0.5);
    pointer-events: none;
  }
  .hue {
    appearance: none;
    height: 10px;
    margin: 0;
    border-radius: 5px;
    background: linear-gradient(to right, red, yellow, lime, cyan, blue, magenta, red);
  }
  .hue::-webkit-slider-thumb {
    appearance: none;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 0 0 1px rgb(0 0 0 / 0.5);
  }
  .hex {
    font: 12px ui-monospace, monospace;
    padding: 3px 5px;
    border: 1px solid rgb(0 0 0 / 0.25);
    border-radius: 4px;
    color: #222;
    background: #fff;
  }

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
