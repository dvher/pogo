package notefile

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/dvher/pogo/pkg/palette"
	"github.com/dvher/pogo/pkg/store"
)

// md renders like the desktop editor (markdown-it with breaks and linkify):
// GFM with hard line breaks. Raw HTML in notes is escaped, not rendered.
var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithHardWraps()),
)

var page = template.Must(template.New("note").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="generator" content="Pogo">
<title>{{.Title}}</title>
<style>
:root { --bg: {{.BG}}; --accent: {{.Accent}}; }
* { box-sizing: border-box; }
body {
  margin: 0; background: #fff; color: #1f1f1f;
  font: 16px/1.55 system-ui, -apple-system, "Segoe UI", Roboto, Cantarell, sans-serif;
  -webkit-print-color-adjust: exact; print-color-adjust: exact;
}
main { max-width: 46rem; margin: 0 auto; padding: 2.5rem 1.5rem; border-top: 0.6rem solid var(--bg); }
h1, h2, h3, h4 { line-height: 1.25; margin: 1.4em 0 0.5em; }
main > :first-child { margin-top: 0; }
a { color: inherit; text-decoration-color: var(--accent); }
code, pre { font-family: ui-monospace, "JetBrains Mono", Menlo, Consolas, monospace; font-size: 0.9em; }
code { background: #f2f2f2; padding: 0.1em 0.3em; border-radius: 4px; }
pre { background: #f5f5f5; padding: 0.8em 1em; border-radius: 6px; overflow-x: auto; white-space: pre-wrap; }
pre code { background: none; padding: 0; }
blockquote { margin: 1em 0; padding: 0 1em; border-left: 4px solid var(--accent); color: #555; }
hr { border: 0; border-top: 1px solid #ddd; margin: 1.5em 0; }
table { border-collapse: collapse; }
th, td { border: 1px solid #ddd; padding: 0.3em 0.6em; }
li:has(> input[type=checkbox]) { list-style: none; margin-left: -1.3em; }
input[type=checkbox] { accent-color: var(--accent); margin: 0 0.45em 0 0; vertical-align: -0.1em; }
img { max-width: 100%; }
@page { margin: 2cm; }
@media print { main { max-width: none; padding: 1rem 0 0; } }
</style>
</head>
<body>
<main>
{{.Body}}
</main>
</body>
</html>
`))

func hex(c [3]uint8) string { return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]) }

// HTML returns the note as a standalone web page, styled for reading and
// printing. Frontmatter is left out.
func HTML(n store.Note) ([]byte, error) {
	var body bytes.Buffer
	if err := md.Convert([]byte(Body(n.Content)), &body); err != nil {
		return nil, err
	}
	c := palette.Get(n.Color)
	var out bytes.Buffer
	err := page.Execute(&out, map[string]any{
		"Title":  Title(n.Content),
		"BG":     template.CSS(hex(c.BG)),
		"Accent": template.CSS(hex(c.Accent)),
		"Body":   template.HTML(body.String()),
	})
	return out.Bytes(), err
}
