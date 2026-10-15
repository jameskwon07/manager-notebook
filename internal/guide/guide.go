// Package guide renders the bundled Markdown guides to HTML once at start-up.
//
// guide/index.json lists the pages as [{"slug","title","order","file"}]; the
// page source is guide/<file> (or guide/<slug>.md when "file" is empty).
// Raw HTML in Markdown is not rendered (goldmark default).
package guide

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Entry is one row of the guide list.
type Entry struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Order int    `json:"order"`
}

// Page is a rendered guide page.
type Page struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	HTML  string `json:"html"`
}

var (
	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	fileRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}\.md$`)
)

type indexEntry struct {
	Entry
	File string `json:"file"`
}

// ValidSlug reports whether s is a safe slug.
func ValidSlug(s string) bool { return slugRe.MatchString(s) }

// Guide holds every rendered page.
type Guide struct {
	list  []Entry
	pages map[string]Page
}

// ErrNotFound is returned for unknown slugs.
var ErrNotFound = errors.New("guide not found")

// Render converts Markdown to HTML with GFM (tables, task lists, strikethrough,
// autolinks). Raw HTML is omitted.
func Render(src []byte) (string, error) {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Load reads dir/index.json and dir/<slug>.md from fsys.
// A missing index gives an empty guide. Entries with bad slugs or missing
// files are skipped and reported in the returned warnings.
func Load(fsys fs.FS, dir string) (*Guide, []string, error) {
	g := &Guide{list: []Entry{}, pages: map[string]Page{}}
	raw, err := fs.ReadFile(fsys, path.Join(dir, "index.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return g, []string{"guide/index.json not found"}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var entries []indexEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return g, []string{"guide/index.json is not a JSON array: " + err.Error()}, nil
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Order < entries[j].Order })
	var warns []string
	for _, e := range entries {
		if !ValidSlug(e.Slug) {
			warns = append(warns, fmt.Sprintf("skip bad slug %q", e.Slug))
			continue
		}
		if _, dup := g.pages[e.Slug]; dup {
			warns = append(warns, fmt.Sprintf("skip duplicate slug %q", e.Slug))
			continue
		}
		file := e.File
		if file == "" {
			file = e.Slug + ".md"
		}
		if !fileRe.MatchString(file) || strings.Contains(file, "..") {
			warns = append(warns, fmt.Sprintf("skip %q: bad file name %q", e.Slug, file))
			continue
		}
		src, err := fs.ReadFile(fsys, path.Join(dir, file))
		if err != nil {
			warns = append(warns, fmt.Sprintf("skip %q: %v", e.Slug, err))
			continue
		}
		html, err := Render(src)
		if err != nil {
			warns = append(warns, fmt.Sprintf("skip %q: %v", e.Slug, err))
			continue
		}
		g.list = append(g.list, e.Entry)
		g.pages[e.Slug] = Page{Slug: e.Slug, Title: e.Title, HTML: html}
	}
	return g, warns, nil
}

// List returns the entries in index order.
func (g *Guide) List() []Entry {
	if g == nil {
		return []Entry{}
	}
	return g.list
}

// Get returns one rendered page.
func (g *Guide) Get(slug string) (Page, error) {
	if g == nil || !ValidSlug(slug) {
		return Page{}, ErrNotFound
	}
	p, ok := g.pages[slug]
	if !ok {
		return Page{}, ErrNotFound
	}
	return p, nil
}
