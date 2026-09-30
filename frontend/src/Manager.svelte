<script lang="ts">
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { NoteService, SettingsService, SyncService } from '../bindings/github.com/dvher/pogo'
  import type { Settings, Status as SyncStatus } from '../bindings/github.com/dvher/pogo/pkg/pogosync/models'
  import type { Note } from '../bindings/github.com/dvher/pogo/pkg/store/models'
  import { colorOf } from './lib/colors'
  import { titleOf } from './lib/markdown'

  let tab = $state<'notes' | 'sync'>('notes')

  // Notes tab
  let notes = $state<Note[]>([])
  let query = $state('')
  let confirmDelete = $state<string | null>(null)
  let filtered = $derived(
    notes.filter((n) => !query.trim() || n.content.toLowerCase().includes(query.trim().toLowerCase())),
  )

  async function loadNotes() {
    notes = ((await NoteService.List()) ?? []).sort((a, b) => b.updatedAt - a.updatedAt)
  }

  function preview(n: Note) {
    const lines = n.content.split('\n').slice(1).join(' ')
    return lines.replace(/\[[ xX]\]/g, ' ').replace(/[#>*_`~\-[\]]/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 110)
  }

  function when(ms: number) {
    return ms ? new Date(ms).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : 'never'
  }

  async function del(id: string) {
    if (confirmDelete !== id) {
      confirmDelete = id
      setTimeout(() => confirmDelete === id && (confirmDelete = null), 3000)
      return
    }
    confirmDelete = null
    await NoteService.Delete(id)
  }

  // Sync tab
  let form = $state<Settings>({ scheme: 'http', host: '', port: '8080', token: '', enabled: false, intervalSec: 30 })
  let status = $state<SyncStatus>({
    enabled: false,
    syncing: false,
    lastSync: 0,
    lastError: '',
    e2eServer: false,
    e2eLocked: false,
    e2eEnabled: false,
  })
  let message = $state<{ ok: boolean; text: string } | null>(null)
  let busy = $state(false)
  let showToken = $state(false)
  let passphrase = $state('')
  let passphrase2 = $state('')

  async function run(fn: () => Promise<string | void>) {
    busy = true
    message = null
    try {
      const text = await fn()
      if (text) message = { ok: true, text }
    } catch (e) {
      message = { ok: false, text: String(e).replace(/^Error:\s*/, '') }
    } finally {
      busy = false
      status = await SyncService.Status()
    }
  }

  const test = () => run(() => SettingsService.TestConnection(form))
  const save = () =>
    run(async () => {
      form = await SettingsService.Save(form)
      return 'Settings saved.'
    })
  const syncNow = () =>
    run(async () => {
      await SyncService.SyncNow()
      return 'Synced.'
    })
  const enableE2E = () =>
    run(async () => {
      if (!status.e2eServer && passphrase !== passphrase2) throw new Error('Passphrases do not match')
      await SettingsService.EnableE2E(passphrase)
      passphrase = passphrase2 = ''
      return status.e2eServer ? 'Unlocked.' : 'End-to-end encryption is on. Use the same passphrase on your other devices.'
    })
  const disableE2E = () =>
    run(async () => {
      await SettingsService.DisableE2E()
      return 'End-to-end encryption is off.'
    })

  onMount(() => {
    loadNotes()
    SettingsService.Get().then((s) => (form = s))
    SyncService.Status().then((s) => (status = s))
    const offs = [
      Events.On('notes:changed', loadNotes),
      Events.On('note:changed', loadNotes),
      Events.On('sync:status', (ev) => (status = ev.data as SyncStatus)),
    ]
    return () => offs.forEach((off) => off())
  })
</script>

<div class="manager">
  <header>
    <h1>
      <svg viewBox="0 0 64 64" aria-hidden="true"><path d="M8 6h48v38L42 58H8z" fill="#ffd740" stroke="#8d6e00" stroke-width="4" stroke-linejoin="round" /></svg>
      Pogo
    </h1>
    <nav>
      <button class:active={tab === 'notes'} onclick={() => (tab = 'notes')}>Notes <span class="count">{notes.length}</span></button>
      <button class:active={tab === 'sync'} onclick={() => (tab = 'sync')}>
        Sync
        {#if status.enabled}<span class="led" class:bad={!!status.lastError} class:busy={status.syncing}></span>{/if}
      </button>
    </nav>
  </header>

  {#if tab === 'notes'}
    <section class="notes">
      <div class="toolbar">
        <input type="search" placeholder="Search notes…" bind:value={query} />
        <button class="primary" onclick={() => NoteService.Create()}>+ New note</button>
        <button onclick={() => NoteService.ShowAll()}>Show all</button>
        <button onclick={() => NoteService.HideAll()}>Hide all</button>
      </div>

      {#if filtered.length === 0}
        <p class="empty">{notes.length ? 'No notes match your search.' : 'No notes yet — create one!'}</p>
      {/if}

      <ul>
        {#each filtered as n (n.id)}
          <li class:hidden={n.hidden}>
            <span class="chip" style:background={colorOf(n.color).bg}></span>
            <div class="text">
              <strong>{titleOf(n.content)}</strong>
              <span class="preview">{preview(n) || ' '}</span>
              <span class="meta">
                Edited {when(n.updatedAt)}
                {#if n.anchored}· anchored{/if}
                {#if n.hidden}· hidden{/if}
              </span>
            </div>
            <div class="actions">
              {#if n.hidden}
                <button onclick={() => NoteService.Show(n.id)}>Show</button>
              {:else}
                <button onclick={() => NoteService.Hide(n.id)}>Hide</button>
              {/if}
              <button class="danger" onclick={() => del(n.id)}>{confirmDelete === n.id ? 'Confirm?' : 'Delete'}</button>
            </div>
          </li>
        {/each}
      </ul>
    </section>
  {:else}
    <section class="sync">
      <div class="card">
        <h2>Server</h2>
        <p class="hint">
          Point Pogo at your self-hosted <strong>Pogo Pad</strong>. On the server, run
          <code>pogo-pad token create --name my-laptop</code> and paste the token below.
        </p>
        <div class="grid">
          <label>
            Protocol
            <select bind:value={form.scheme}>
              <option value="http">http</option>
              <option value="https">https</option>
            </select>
          </label>
          <label class="grow">
            IP address or hostname
            <input placeholder="192.168.1.10 or notes.example.com" bind:value={form.host} />
          </label>
          <label class="port">
            Port
            <input inputmode="numeric" placeholder="8080" bind:value={form.port} />
          </label>
        </div>
        <label>
          API token
          <span class="row">
            <input type={showToken ? 'text' : 'password'} placeholder="pogo_…" bind:value={form.token} autocomplete="off" />
            <button onclick={() => (showToken = !showToken)}>{showToken ? 'Hide' : 'Show'}</button>
          </span>
        </label>
        <div class="grid">
          <label class="check">
            <input type="checkbox" bind:checked={form.enabled} />
            Enable sync
          </label>
          <label class="interval">
            Every
            <input type="number" min="10" bind:value={form.intervalSec} />
            seconds
          </label>
        </div>
        <div class="buttons">
          <button disabled={busy} onclick={test}>Test connection</button>
          <button class="primary" disabled={busy} onclick={save}>Save</button>
          <button disabled={busy || !status.enabled} onclick={syncNow}>Sync now</button>
        </div>
        {#if message}
          <p class="message" class:ok={message.ok}>{message.text}</p>
        {/if}
        <p class="status">
          {#if !status.enabled}
            Sync is off. Notes are only stored on this computer.
          {:else if status.syncing}
            Syncing…
          {:else if status.lastError}
            <span class="bad">Last sync failed: {status.lastError}</span>
          {:else}
            Last synced {when(status.lastSync)}.
          {/if}
        </p>
      </div>

      <div class="card">
        <h2>End-to-end encryption <span class="tag">optional</span></h2>
        <p class="hint">
          Notes on this computer are always encrypted. With end-to-end encryption, notes are also encrypted
          <em>before</em> they leave this device, so the server only ever stores unreadable data. Every device needs the
          same passphrase, and if it is lost, notes stored on the server cannot be recovered.
        </p>
        {#if !status.enabled}
          <p class="status">Save working server settings first.</p>
        {:else if status.e2eServer && status.e2eEnabled}
          <p class="status ok">🔒 On — this device encrypts everything it syncs.</p>
          <button class="danger" disabled={busy} onclick={disableE2E}>Turn off for all devices</button>
        {:else if status.e2eServer}
          <p class="status bad">This server's notes are end-to-end encrypted. Enter the passphrase to sync.</p>
          <span class="row">
            <input type="password" placeholder="Passphrase" bind:value={passphrase} />
            <button class="primary" disabled={busy || !passphrase} onclick={enableE2E}>Unlock</button>
          </span>
        {:else}
          <div class="grid">
            <input type="password" placeholder="Passphrase (8+ characters)" bind:value={passphrase} />
            <input type="password" placeholder="Repeat passphrase" bind:value={passphrase2} />
          </div>
          <div class="buttons">
            <button class="primary" disabled={busy || passphrase.length < 8} onclick={enableE2E}>Turn on</button>
          </div>
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
  .manager {
    display: flex;
    flex-direction: column;
    height: 100vh;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 20px;
    border-bottom: 1px solid var(--m-border);
    background: var(--m-surface);
  }
  h1 {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0;
    font-size: 17px;
  }
  h1 svg { width: 22px; height: 22px; }
  nav { display: flex; gap: 4px; }
  nav button {
    border: none;
    background: none;
    color: var(--m-muted);
    padding: 6px 12px;
    border-radius: 6px;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 6px;
  }
  nav button.active { background: var(--m-bg); color: var(--m-text); font-weight: 600; }
  .count { font-size: 11px; color: var(--m-muted); }
  .led { width: 8px; height: 8px; border-radius: 50%; background: var(--m-ok); }
  .led.bad { background: var(--m-danger); }
  .led.busy { animation: blink 0.8s infinite alternate; }
  @keyframes blink { to { opacity: 0.3; } }

  section {
    flex: 1;
    overflow-y: auto;
    padding: 16px 20px;
  }

  button {
    border: 1px solid var(--m-border);
    background: var(--m-surface);
    color: var(--m-text);
    padding: 6px 12px;
    border-radius: 6px;
    cursor: pointer;
  }
  button:hover:not(:disabled) { border-color: var(--m-muted); }
  button:disabled { opacity: 0.5; cursor: default; }
  button.primary { background: var(--m-primary); border-color: var(--m-primary); color: var(--m-primary-ink); font-weight: 600; }
  button.danger { color: var(--m-danger); }

  input,
  select {
    font: inherit;
    padding: 6px 8px;
    border: 1px solid var(--m-border);
    border-radius: 6px;
    background: var(--m-bg);
    color: var(--m-text);
    min-width: 0;
  }
  input:focus,
  select:focus { outline: 2px solid var(--m-primary); outline-offset: -1px; }

  .toolbar { display: flex; gap: 8px; margin-bottom: 12px; }
  .toolbar input { flex: 1; }

  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  li {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    background: var(--m-surface);
    border: 1px solid var(--m-border);
    border-radius: 8px;
  }
  li.hidden { opacity: 0.65; }
  .chip { width: 14px; align-self: stretch; border-radius: 3px; flex: none; }
  .text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .text strong, .preview { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .preview { color: var(--m-muted); font-size: 13px; }
  .meta { color: var(--m-muted); font-size: 11px; }
  .actions { display: flex; gap: 6px; flex: none; }
  .empty { color: var(--m-muted); text-align: center; margin-top: 40px; }

  .card {
    background: var(--m-surface);
    border: 1px solid var(--m-border);
    border-radius: 10px;
    padding: 16px;
    margin-bottom: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  h2 { margin: 0; font-size: 15px; display: flex; align-items: center; gap: 8px; }
  .tag { font-size: 11px; font-weight: 500; color: var(--m-muted); border: 1px solid var(--m-border); border-radius: 10px; padding: 1px 7px; }
  .hint { margin: 0; color: var(--m-muted); font-size: 13px; line-height: 1.5; }
  code { font-size: 12px; background: var(--m-bg); padding: 1px 4px; border-radius: 4px; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--m-muted); }
  label input, label select { font-size: 14px; }
  .grid { display: flex; gap: 10px; align-items: flex-end; flex-wrap: wrap; }
  .grid > input { flex: 1; }
  .grow { flex: 1; }
  .port input { width: 90px; }
  .check { flex-direction: row; align-items: center; gap: 6px; font-size: 14px; color: var(--m-text); }
  .interval { flex-direction: row; align-items: center; gap: 6px; font-size: 14px; color: var(--m-text); }
  .interval input { width: 70px; }
  .row { display: flex; gap: 8px; }
  .row input { flex: 1; }
  .buttons { display: flex; gap: 8px; flex-wrap: wrap; }
  .message { margin: 0; padding: 8px 10px; border-radius: 6px; font-size: 13px; background: color-mix(in srgb, var(--m-danger) 12%, transparent); color: var(--m-danger); }
  .message.ok { background: color-mix(in srgb, var(--m-ok) 12%, transparent); color: var(--m-ok); }
  .status { margin: 0; font-size: 13px; color: var(--m-muted); }
  .status.ok { color: var(--m-ok); }
  .bad { color: var(--m-danger); }
</style>
