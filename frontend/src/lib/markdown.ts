import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'

const md = new MarkdownIt({ html: false, linkify: true, breaks: true })

const taskPrefix = /^\[([ xX])\](?:[ \t]+|$)/

// Turns "- [ ] text" list items into checkboxes that remember their source
// line, so clicking one can toggle the Markdown.
md.core.ruler.after('text_join', 'task_lists', (state) => {
  const tokens = state.tokens
  for (let i = 2; i < tokens.length; i++) {
    const inline = tokens[i]
    if (inline.type !== 'inline' || tokens[i - 1].type !== 'paragraph_open' || tokens[i - 2].type !== 'list_item_open') continue
    const m = taskPrefix.exec(inline.content)
    const first = inline.children?.[0]
    if (!m || !inline.map || !first || first.type !== 'text' || !taskPrefix.test(first.content)) continue

    const done = m[1] !== ' '
    const line = inline.map[0]
    first.content = first.content.replace(taskPrefix, '')
    const box = new state.Token('html_inline', '', 0)
    box.content = `<input type="checkbox" class="task-box" tabindex="-1"${done ? ' checked' : ''}>`
    inline.children!.unshift(box)

    const li = tokens[i - 2]
    li.attrJoin('class', done ? 'task done' : 'task')
    li.attrSet('data-line', String(line))
  }
})

export function render(src: string): string {
  return DOMPurify.sanitize(md.render(src))
}

const taskLine = /^(\s*(?:>\s*)*(?:[-*+]|\d+[.)])\s+\[)([ xX])(\])/

// Flips the checkbox on the given 0-based source line.
export function toggleTask(src: string, line: number): string {
  const lines = src.split('\n')
  const l = lines[line]
  if (l === undefined || !taskLine.test(l)) return src
  lines[line] = l.replace(taskLine, (_, pre, mark, post) => pre + (mark === ' ' ? 'x' : ' ') + post)
  return lines.join('\n')
}

// First meaningful line, for titles in the manager.
export function titleOf(src: string): string {
  const line = src.split('\n').map((l) => l.replace(/^\s*(#+|[-*+]|\d+[.)])\s*(\[[ xX]\]\s*)?/, '').trim()).find(Boolean)
  return line || 'Empty note'
}
