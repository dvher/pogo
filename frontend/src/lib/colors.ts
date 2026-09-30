// Keep in sync with noteColors in windows.go.
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

export function colorOf(name: string) {
  return colors[name] ?? colors.yellow
}
