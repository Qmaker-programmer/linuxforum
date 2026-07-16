// Copyright (C) 2026 Qmaker <andresavalosgallegos@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var mdParser goldmark.Markdown
var htmlSanitizer *bluemonday.Policy

func init() {
	mdParser = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)

	htmlSanitizer = bluemonday.UGCPolicy()
	htmlSanitizer.AllowElements("span")
	htmlSanitizer.AllowAttrs("class").Matching(regexp.MustCompile(`[\w\s\-]+`)).OnElements("span")
}

func renderMarkdown(markdown string) template.HTML {
	if markdown == "" {
		return ""
	}

	var buf bytes.Buffer
	if err := mdParser.Convert([]byte(markdown), &buf); err != nil {
		return ""
	}

	sanitized := htmlSanitizer.Sanitize(buf.String())
	return template.HTML(sanitized)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s,
		"'", "&#39;")
	return s
}
