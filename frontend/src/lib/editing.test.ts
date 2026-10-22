import { describe, expect, it } from 'vitest'
import {
  autopair, continueList, indent, link, outdent, pasteChecklist, tab, toChecklist, toggleTask, unpair, wrap,
  type Edit,
} from './editing'

// Runs a command on text where "|" marks the cursor, or "<" and ">" mark a
// selection, and returns the result in the same form.
function run(src: string, cmd: (t: string, s: number, e: number) => Edit | null): string | null {
  let s = src.indexOf('<')
  let e: number
  if (s >= 0) {
    e = src.indexOf('>') - 1
    src = src.replace('<', '').replace('>', '')
  } else {
    s = e = src.indexOf('|')
    src = src.replace('|', '')
  }
  const ed = cmd(src, s, e)
  if (!ed) return null
  const out = src.slice(0, ed.from) + ed.insert + src.slice(ed.to)
  if (ed.selStart === ed.selEnd) return out.slice(0, ed.selStart) + '|' + out.slice(ed.selStart)
  return out.slice(0, ed.selStart) + '<' + out.slice(ed.selStart, ed.selEnd) + '>' + out.slice(ed.selEnd)
}

describe('wrap', () => {
  const bold = (t: string, s: number, e: number) => wrap(t, s, e, '**')
  it('wraps and unwraps the selection', () => {
    expect(run('a <word> b', bold)).toBe('a **<word>** b')
    expect(run('a **<word>** b', bold)).toBe('a <word> b')
    expect(run('a <**word**> b', bold)).toBe('a <word> b')
  })
  it('inserts markers around the cursor', () => {
    expect(run('a | b', bold)).toBe('a **|** b')
    expect(run('a **|** b', bold)).toBe('a | b')
  })
})

describe('link', () => {
  it('selects the url to fill in', () => {
    expect(run('see <docs>', link)).toBe('see [docs](<url>)')
    expect(run('|', link)).toBe('[](<url>)')
  })
  it('puts a selected url in the target', () => {
    expect(run('<https://pogo.dev>', link)).toBe('[|](https://pogo.dev)')
  })
})

describe('toggleTask', () => {
  it('turns lines into tasks and back', () => {
    expect(run('milk|', toggleTask)).toBe('- [ ] milk|')
    expect(run('  - milk|', toggleTask)).toBe('  - [ ] milk|')
    expect(run('1. milk|', toggleTask)).toBe('1. [ ] milk|')
    expect(run('- [x] milk|', toggleTask)).toBe('milk|')
    expect(run('|', toggleTask)).toBe('- [ ] |')
  })
  it('works on every selected line', () => {
    expect(run('<a\n\n- b\n- [ ] c>', toggleTask)).toBe('- [ ] <a\n\n- [ ] b\n- [ ] c>')
    expect(run('<- [ ] a\n- [x] b>', toggleTask)).toBe('<a\nb>')
    expect(run('<a\n>b', toggleTask)).toBe('- [ ] <a\n>b')
  })
})

describe('continueList', () => {
  it('starts the next item', () => {
    expect(run('- milk|', continueList)).toBe('- milk\n- |')
    expect(run('  * [x] milk|', continueList)).toBe('  * [x] milk\n  * [ ] |')
    expect(run('9) one|', continueList)).toBe('9) one\n10) |')
    expect(run('> - quoted|', continueList)).toBe('> - quoted\n> - |')
    expect(run('- mi|lk', continueList)).toBe('- mi\n- |lk')
  })
  it('ends the list on an empty item', () => {
    expect(run('- a\n- [ ] |', continueList)).toBe('- a\n|')
    expect(run('- a\n- |', continueList)).toBe('- a\n|')
  })
  it('leaves other lines alone', () => {
    expect(run('plain|', continueList)).toBeNull()
    expect(run('|- a', continueList)).toBeNull()
    expect(run('<- a>', continueList)).toBeNull()
  })
})

describe('indent', () => {
  it('indents and outdents lines', () => {
    expect(run('- a|', indent)).toBe('  - a|')
    expect(run('    - a|', outdent)).toBe('  - a|')
    expect(run('\t- a|', outdent)).toBe('- a|')
    expect(run('a|', outdent)).toBeNull()
    expect(run('<- a\n- b>', indent)).toBe('  <- a\n  - b>')
  })
  it('tab indents list items and inserts spaces elsewhere', () => {
    expect(run('- a|', tab)).toBe('  - a|')
    expect(run('a|b', tab)).toBe('a  |b')
  })
})

describe('autopair', () => {
  const type = (ch: string) => (t: string, s: number, e: number) => autopair(t, s, e, ch)
  it('closes brackets and quotes', () => {
    expect(run('a |', type('['))).toBe('a [|]')
    expect(run('|', type('('))).toBe('(|)')
    expect(run('|', type('"'))).toBe('"|"')
  })
  it('steps over the closer', () => {
    expect(run('[a|]', type(']'))).toBe('[a]|')
    expect(run('"a|"', type('"'))).toBe('"a"|')
  })
  it('wraps the selection', () => {
    expect(run('<a b>', type('['))).toBe('[<a b>]')
  })
  it('does not pair next to words', () => {
    expect(run('|word', type('['))).toBeNull()
    expect(run('don|', type('"'))).toBeNull()
    expect(run('|', type(']'))).toBeNull()
  })
  it('backspace removes an empty pair', () => {
    expect(run('a [|]', unpair)).toBe('a |')
    expect(run('a [b|]', unpair)).toBeNull()
  })
})

// Same cases as ChecklistFromText in Pogo Pocket's core/markdown_test.go.
const checklistCases: [string, string][] = [
  ['milk\neggs', '- [ ] milk\n- [ ] eggs'],
  ['• milk\n· eggs\n▪ ham\n◦ jam\n– tea\n— gin', '- [ ] milk\n- [ ] eggs\n- [ ] ham\n- [ ] jam\n- [ ] tea\n- [ ] gin'],
  ['- milk\n* eggs\n+ ham\n1. jam\n2) tea', '- [ ] milk\n- [ ] eggs\n- [ ] ham\n- [ ] jam\n- [ ] tea'],
  ['☐ milk\n□ eggs\n☑ ham\n☒ jam\n✓ tea\n✔ gin\n✅ rum', '- [ ] milk\n- [ ] eggs\n- [x] ham\n- [x] jam\n- [x] tea\n- [x] gin\n- [x] rum'],
  ['- [ ] milk\n- [x] eggs\n[X] ham', '- [ ] milk\n- [x] eggs\n- [x] ham'],
  ['Groceries\n  milk\n\tsoy\n\n\r\neggs  ', '- [ ] Groceries\n  - [ ] milk\n  - [ ] soy\n- [ ] eggs'],
  ['-5 degrees\n1.5 kg', '- [ ] -5 degrees\n- [ ] 1.5 kg'],
  ['\n  \n', ''],
]

describe('toChecklist', () => {
  it.each(checklistCases)('%j', (src, want) => {
    expect(toChecklist(src)).toBe(want)
  })

  it('pastes on lines of its own', () => {
    const paste = (clip: string) => (t: string, s: number, e: number) => pasteChecklist(t, s, e, clip)
    expect(run('|', paste('a\nb'))).toBe('- [ ] a\n- [ ] b|')
    expect(run('Todo|', paste('a'))).toBe('Todo\n- [ ] a|')
    expect(run('x|y', paste('a'))).toBe('x\n- [ ] a|\ny')
    expect(run('x\n<old>\ny', paste('a'))).toBe('x\n- [ ] a|\ny')
    expect(run('|', paste('  '))).toBeNull()
  })
})
