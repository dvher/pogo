// Keep in sync with pkg/palette/palette.go.
export const colors: Record<string, { bg: string; ink: string; accent: string }> = {
  yellow: { bg: '#fff176', ink: '#3e3500', accent: '#c7b200' },
  pink:   { bg: '#f8bbd0', ink: '#4a1027', accent: '#d0668c' },
  green:  { bg: '#c5e1a5', ink: '#1f3a0b', accent: '#6f9e3f' },
  blue:   { bg: '#b3e5fc', ink: '#0b3346', accent: '#4a9cc2' },
  purple: { bg: '#d1c4e9', ink: '#2b1a4d', accent: '#7e67b3' },
  orange: { bg: '#ffcc80', ink: '#4a2800', accent: '#d08a24' },
  gray:   { bg: '#e0e0e0', ink: '#262626', accent: '#8a8a8a' },
}

export const colorNames = Object.keys(colors)

export type Color = { bg: string; ink: string; accent: string }

const custom = /^#[0-9a-f]{6}$/

// Named colors, or any "#rrggbb" with derived text and accent colors.
export function colorOf(name: string): Color {
  if (colors[name]) return colors[name]
  if (custom.test(name)) return derive(name)
  return colors.yellow
}

export const isCustom = (name: string) => custom.test(name)

type RGB = [number, number, number]

const toHex = (c: RGB) => '#' + c.map((v) => v.toString(16).padStart(2, '0')).join('')

// Mirrors Derive in pkg/palette/palette.go (checked by testdata/derive.json).
export function derive(bg: string): Color {
  const rgb = parseHex(bg)
  const [h, s, l] = toHSL(rgb)
  const lum = luminance(rgb)
  const dark = lum > 0.179
  let [inkL, step, accentL] = dark ? [0.15, -0.01, l * 0.6] : [0.92, 0.01, l + (1 - l) * 0.45]
  const inkS = Math.min(s, 0.6)
  let ink = fromHSL(h, inkS, inkL)
  while (contrast(lum, luminance(ink)) < 4.5 && inkL > 0 && inkL < 1) {
    inkL = Math.round((inkL + step) * 100) / 100
    ink = fromHSL(h, inkS, inkL)
  }
  return { bg, ink: toHex(ink), accent: toHex(fromHSL(h, s, accentL)) }
}

export function toHSL([r8, g8, b8]: RGB): RGB {
  const [r, g, b] = [r8 / 255, g8 / 255, b8 / 255]
  const hi = Math.max(r, g, b)
  const lo = Math.min(r, g, b)
  const l = (hi + lo) / 2
  const d = hi - lo
  if (d === 0) return [0, 0, l]
  const s = d / (1 - Math.abs(2 * l - 1))
  let h: number
  if (hi === r) h = (((g - b) / d + 6) % 6)
  else if (hi === g) h = (b - r) / d + 2
  else h = (r - g) / d + 4
  return [h * 60, s, l]
}

export function fromHSL(h: number, s: number, l: number): RGB {
  const c = (1 - Math.abs(2 * l - 1)) * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = l - c / 2
  const [r, g, b] =
    h < 60 ? [c, x, 0] : h < 120 ? [x, c, 0] : h < 180 ? [0, c, x] : h < 240 ? [0, x, c] : h < 300 ? [x, 0, c] : [c, 0, x]
  const ch = (v: number) => Math.round(Math.min(1, Math.max(0, v + m)) * 255)
  return [ch(r), ch(g), ch(b)]
}

export const hslHex = (h: number, s: number, l: number) => toHex(fromHSL(h, s, l))

export const parseHex = (hex: string) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16)) as RGB

// Whether a color gets light text (the same test derive uses).
export const isDark = (c: Color) => luminance(parseHex(c.bg)) <= 0.179

function luminance(c: RGB) {
  const lin = (v: number) => {
    const f = v / 255
    return f <= 0.04045 ? f / 12.92 : ((f + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * lin(c[0]) + 0.7152 * lin(c[1]) + 0.0722 * lin(c[2])
}

const contrast = (a: number, b: number) => (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)
