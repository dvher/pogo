import { describe, expect, it } from 'vitest'
import { colorNames, colorOf, colors } from './colors'

describe('colors', () => {
  it('lists yellow first, as the default', () => {
    expect(colorNames[0]).toBe('yellow')
    expect(colorNames).toHaveLength(7)
  })

  it('falls back to yellow for unknown colors', () => {
    expect(colorOf('pink')).toBe(colors.pink)
    expect(colorOf('')).toBe(colors.yellow)
    expect(colorOf('chartreuse')).toBe(colors.yellow)
  })

  it('uses 6-digit hex colors', () => {
    for (const c of Object.values(colors)) {
      for (const v of [c.bg, c.ink, c.accent]) expect(v).toMatch(/^#[0-9a-f]{6}$/)
    }
  })
})
