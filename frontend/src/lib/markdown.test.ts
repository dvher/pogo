// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'
import { render, titleOf, toggleTask } from './markdown'

describe('task lists', () => {
  const src = '# Groceries\n\n- [ ] milk\n- [x] eggs\n  - [ ] nested\n1. [ ] numbered\n- plain item'

  it('renders checkboxes with source lines', () => {
    const div = document.createElement('div')
    div.innerHTML = render(src)
    const tasks = [...div.querySelectorAll<HTMLElement>('li.task')]
    expect(tasks.map((t) => t.dataset.line)).toEqual(['2', '3', '4', '5'])
    expect(tasks[1].classList.contains('done')).toBe(true)
    expect(tasks[0].querySelector('input')?.checked).toBe(false)
    expect(tasks[0].textContent?.trim()).toBe('milk')
    expect(div.querySelectorAll('li:not(.task)').length).toBe(1)
  })

  it('toggles the right line', () => {
    expect(toggleTask(src, 2)).toContain('- [x] milk')
    expect(toggleTask(src, 3)).toContain('- [ ] eggs')
    expect(toggleTask(src, 4)).toContain('  - [x] nested')
    expect(toggleTask(src, 5)).toContain('1. [x] numbered')
    expect(toggleTask(src, 6)).toBe(src) // not a task
  })

  it('toggles tasks in quotes and keeps CRLF line endings', () => {
    expect(toggleTask('> - [ ] quoted', 0)).toBe('> - [x] quoted')
    expect(toggleTask('* [X] star', 0)).toBe('* [ ] star')
    expect(toggleTask('- [ ] a\r\n- [ ] b', 1)).toBe('- [ ] a\r\n- [x] b')
  })

  it('ignores lines that do not exist', () => {
    expect(toggleTask(src, -1)).toBe(src)
    expect(toggleTask(src, 99)).toBe(src)
  })

  it('does not treat brackets mid-text as tasks', () => {
    const div = document.createElement('div')
    div.innerHTML = render('- see [ ] later')
    expect(div.querySelector('li.task')).toBeNull()
  })
})

describe('sanitizing', () => {
  it('escapes raw HTML and drops javascript links', () => {
    const div = document.createElement('div')
    div.innerHTML = render('<img src=x onerror=alert(1)>\n\n[x](javascript:alert(1))')
    expect(div.querySelector('img')).toBeNull()
    expect(div.querySelector('a')).toBeNull()
  })
})

describe('titleOf', () => {
  it('uses the first meaningful line', () => {
    expect(titleOf('\n# Hello\nworld')).toBe('Hello')
    expect(titleOf('- [ ] buy milk')).toBe('buy milk')
    expect(titleOf('')).toBe('Empty note')
  })
})
