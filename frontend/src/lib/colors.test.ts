import { describe, expect, it } from 'vitest'
import { colorNames, colorOf, colors, derive, fromHSL, toHSL } from './colors'
import fixture from '../../../pkg/palette/testdata/derive.json'

describe('colors', () => {
  it('lists yellow first, as the default', () => {
    expect(colorNames[0]).toBe('yellow')
    expect(colorNames).toHaveLength(7)
  })

  it('falls back to yellow for unknown colors', () => {
    expect(colorOf('pink')).toBe(colors.pink)
    expect(colorOf('')).toBe(colors.yellow)
    expect(colorOf('chartreuse')).toBe(colors.yellow)
    expect(colorOf('#FFF176')).toBe(colors.yellow)
  })

  it('derives text and accent like pkg/palette', () => {
    for (const [bg, ink, accent] of fixture) {
      expect(colorOf(bg)).toEqual({ bg, ink, accent })
    }
  })

  it('round-trips HSL', () => {
    for (const hex of ['#3a7bd5', '#000000', '#ffffff', '#c0ffee']) {
      const rgb = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16)) as [number, number, number]
      expect(fromHSL(...toHSL(rgb))).toEqual(rgb)
    }
    expect(derive('#000000').ink).not.toBe(derive('#ffffff').ink)
  })

  it('uses 6-digit hex colors', () => {
    for (const c of Object.values(colors)) {
      for (const v of [c.bg, c.ink, c.accent]) expect(v).toMatch(/^#[0-9a-f]{6}$/)
    }
  })
})
