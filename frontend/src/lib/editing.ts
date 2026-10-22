// Markdown editing commands for the note textarea. Each one is a pure
// function of the text and selection that returns the change to make (or
// null to let the key do its default thing), so they are easy to test and
// the caller can apply them in a way that keeps the native undo stack.

export interface Edit {
  from: number
  to: number
  insert: string
  // Selection after the edit, in the new text.
  selStart: number
  selEnd: number
}

const lineStart = (text: string, pos: number) => text.lastIndexOf('\n', pos - 1) + 1

function lineEnd(text: string, pos: number) {
  const i = text.indexOf('\n', pos)
  return i < 0 ? text.length : i
}

// Applies fn to every line the selection touches. A selection ending right
// after a newline does not include the next line.
function mapLines(text: string, s: number, e: number, fn: (line: string, i: number, lines: string[]) => string): Edit | null {
  const from = lineStart(text, s)
  const to = lineEnd(text, e > s && text[e - 1] === '\n' ? e - 1 : e)
  const lines = text.slice(from, to).split('\n')
  const out = lines.map((l, i) => fn(l, i, lines))
  const insert = out.join('\n')
  if (insert === text.slice(from, to)) return null
  const firstDelta = out[0].length - lines[0].length
  const selStart = Math.max(from, s + firstDelta)
  const selEnd = s === e ? selStart : Math.max(selStart, e + insert.length - (to - from))
  return { from, to, insert, selStart, selEnd }
}

// Wraps the selection in marker (** for bold, * for italic, …), or unwraps
// it when it is already wrapped.
export function wrap(text: string, s: number, e: number, marker: string): Edit {
  const n = marker.length
  const sel = text.slice(s, e)
  if (sel.length >= 2 * n && sel.startsWith(marker) && sel.endsWith(marker)) {
    const inner = sel.slice(n, -n)
    return { from: s, to: e, insert: inner, selStart: s, selEnd: s + inner.length }
  }
  if (text.slice(s - n, s) === marker && text.slice(e, e + n) === marker) {
    return { from: s - n, to: e + n, insert: sel, selStart: s - n, selEnd: e - n }
  }
  return { from: s, to: e, insert: marker + sel + marker, selStart: s + n, selEnd: e + n }
}

// Turns the selection into a link and selects the part still to fill in.
export function link(text: string, s: number, e: number): Edit {
  const sel = text.slice(s, e)
  if (/^https?:\/\/\S+$/.test(sel)) {
    return { from: s, to: e, insert: `[](${sel})`, selStart: s + 1, selEnd: s + 1 }
  }
  const at = s + sel.length + 3
  return { from: s, to: e, insert: `[${sel}](url)`, selStart: at, selEnd: at + 3 }
}

// indent, quotes, list marker, spacing, checkbox
const listItem = /^(\s*)((?:>\s?)*)([-*+]|\d+[.)])(\s+|$)(\[[ xX]\](?:\s+|$))?/
const isTask = (l: string) => !!listItem.exec(l)?.[5]

// Makes the touched lines tasks, or plain text again if they all are.
export function toggleTask(text: string, s: number, e: number): Edit | null {
  return mapLines(text, s, e, (l, _, lines) => {
    if (lines.length > 1 && !l.trim()) return l
    const filled = lines.filter((x) => x.trim())
    if (filled.length && filled.every(isTask)) {
      const m = listItem.exec(l)!
      return m[1] + m[2] + l.slice(m[0].length)
    }
    const m = listItem.exec(l)
    if (m?.[5]) return l
    if (m) return l.slice(0, m[0].length) + (m[4] ? '' : ' ') + '[ ] ' + l.slice(m[0].length)
    const indent = /^\s*/.exec(l)![0]
    return indent + '- [ ] ' + l.slice(indent.length)
  })
}

// Enter inside a list item starts the next one; Enter on an empty item ends
// the list.
export function continueList(text: string, s: number, e: number): Edit | null {
  if (s !== e) return null
  const from = lineStart(text, s)
  const to = lineEnd(text, s)
  const line = text.slice(from, to)
  const m = listItem.exec(line)
  if (!m || s < from + m[0].length) return null
  if (!line.slice(m[0].length).trim()) {
    return { from, to, insert: '', selStart: from, selEnd: from }
  }
  let marker = m[3]
  const num = /^(\d+)([.)])$/.exec(marker)
  if (num) marker = Number(num[1]) + 1 + num[2]
  const insert = '\n' + m[1] + m[2] + marker + ' ' + (m[5] ? '[ ] ' : '')
  return { from: s, to: s, insert, selStart: s + insert.length, selEnd: s + insert.length }
}

export function indent(text: string, s: number, e: number): Edit | null {
  return mapLines(text, s, e, (l, _, lines) => (lines.length > 1 && !l.trim() ? l : '  ' + l))
}

export function outdent(text: string, s: number, e: number): Edit | null {
  return mapLines(text, s, e, (l) => l.replace(/^(?: {1,2}|\t)/, ''))
}

// Tab indents list items and multi-line selections, and otherwise inserts
// two spaces.
export function tab(text: string, s: number, e: number): Edit | null {
  const multi = text.slice(s, e).includes('\n')
  if (multi || listItem.test(text.slice(lineStart(text, s), lineEnd(text, s)))) return indent(text, s, e)
  return { from: s, to: e, insert: '  ', selStart: s + 2, selEnd: s + 2 }
}

const pairs: Record<string, string> = { '(': ')', '[': ']', '{': '}', '`': '`', '"': '"' }
const closers = new Set(Object.values(pairs))

// Typing an opening bracket or quote adds the closing one (or wraps the
// selection); typing a closer that is already there steps over it.
export function autopair(text: string, s: number, e: number, ch: string): Edit | null {
  if (s === e && closers.has(ch) && text[s] === ch) {
    return { from: s, to: s, insert: '', selStart: s + 1, selEnd: s + 1 }
  }
  const close = pairs[ch]
  if (!close) return null
  if (s !== e) {
    return { from: s, to: e, insert: ch + text.slice(s, e) + close, selStart: s + 1, selEnd: e + 1 }
  }
  if (/\w/.test(text[s] ?? '')) return null
  if (ch === close && /\w/.test(text[s - 1] ?? '')) return null
  return { from: s, to: s, insert: ch + close, selStart: s + 1, selEnd: s + 1 }
}

// Backspace between an empty pair deletes both halves.
export function unpair(text: string, s: number, e: number): Edit | null {
  if (s !== e || s === 0 || pairs[text[s - 1]] !== text[s]) return null
  return { from: s - 1, to: s + 1, insert: '', selStart: s - 1, selEnd: s - 1 }
}

const bullet = /^(?:[-*+]\s+|[•·▪◦–—]\s*|\d+[.)]\s+)/
const checkMark = /^(?:\[([ xX])\]|([☐□])|([☑☒✓✔✅]))\s*/

// Turns pasted lines ("• milk", "☑ eggs", "1. bread", plain text…) into a
// Markdown checklist, keeping indentation and checked state. Keep in sync
// with ChecklistFromText in the Pogo Pocket core.
export function toChecklist(src: string): string {
  const out: string[] = []
  for (const raw of src.split(/\r?\n|\r/)) {
    if (!raw.trim()) continue
    const indent = /^[ \t]*/.exec(raw)![0].replace(/\t/g, '  ')
    let body = raw.trim().replace(bullet, '')
    let done = false
    const m = checkMark.exec(body)
    if (m) {
      done = (m[1] !== undefined && m[1] !== ' ') || m[3] !== undefined
      body = body.slice(m[0].length)
    }
    out.push(`${indent}- [${done ? 'x' : ' '}] ${body.trim()}`)
  }
  return out.join('\n')
}

// Replaces the selection with the clipboard text as a checklist, on lines
// of its own.
export function pasteChecklist(text: string, s: number, e: number, clip: string): Edit | null {
  let insert = toChecklist(clip)
  if (!insert) return null
  if (s > 0 && text[s - 1] !== '\n') insert = '\n' + insert
  const end = s + insert.length
  if (e < text.length && text[e] !== '\n') insert += '\n'
  return { from: s, to: e, insert, selStart: end, selEnd: end }
}
