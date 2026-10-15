// Package assets embeds the web screen and the guide pages into the binary,
// so the image needs no other files and makes no outside requests.
package assets

import "embed"

// FS holds web/ (index.html, app.js, app.css, content.json, favicon.svg)
// and guide/ (index.json and *.md).
//
//go:embed web guide
var FS embed.FS
