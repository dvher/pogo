<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import DOMPurify from 'dompurify'
  import { NoteService } from '../bindings/github.com/dvher/pogo'

  let { id }: { id: string } = $props()

  let body = $state('')
  let error = $state('')

  // The page comes from notefile.HTML: take its styles and content and show
  // them here, so the print dialog prints just the note.
  onMount(() => {
    const done = () => Window.Close()
    window.addEventListener('afterprint', done)
    NoteService.PrintHTML(id)
      .then(async (html) => {
        const doc = new DOMParser().parseFromString(html, 'text/html')
        document.title = doc.title
        for (const style of doc.querySelectorAll('style')) document.head.append(style)
        body = DOMPurify.sanitize(doc.querySelector('main')?.innerHTML ?? '')
        await tick()
        window.print()
      })
      .catch((e) => (error = String(e)))
    return () => window.removeEventListener('afterprint', done)
  })
</script>

<div class="bar">
  <button class="primary" onclick={() => window.print()}>Print / Save as PDF</button>
  <button onclick={() => Window.Close()}>Close</button>
</div>
{#if error}<p class="error">{error}</p>{/if}
<main>{@html body}</main>

<style>
  .bar {
    position: sticky;
    top: 0;
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 8px 12px;
    background: #fafafa;
    border-bottom: 1px solid #ddd;
  }
  .bar button {
    font: 13px system-ui, sans-serif;
    padding: 5px 12px;
    border: 1px solid #ccc;
    border-radius: 6px;
    background: #fff;
    color: #222;
    cursor: pointer;
  }
  .bar .primary { background: #222; border-color: #222; color: #fff; }
  .error { color: #b00020; padding: 0 1.5rem; }
  @media print {
    .bar { display: none; }
  }
</style>
