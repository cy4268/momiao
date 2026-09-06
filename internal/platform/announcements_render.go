package platform

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var ErrAnnouncementInvalid = errors.New("ANNOUNCEMENT_INVALID")

type RenderedAnnouncement struct {
	HTML          string `json:"sanitized_html"`
	MarkdownHash  string `json:"body_markdown_hash"`
	HTMLHash      string `json:"sanitized_html_hash"`
	PolicyVersion string `json:"sanitizer_policy_version"`
}

func RenderAnnouncement(markdown string) (RenderedAnnouncement, error) {
	if len(markdown) > 32768 || !utf8.ValidString(markdown) || strings.TrimSpace(markdown) == "" || strings.ContainsRune(markdown, 0) {
		return RenderedAnnouncement{}, ErrAnnouncementInvalid
	}
	source := []byte(markdown)
	md := goldmark.New()
	doc := md.Parser().Parse(text.NewReader(source))
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.RawHTML, *ast.HTMLBlock, *ast.Image:
			return ast.WalkStop, ErrAnnouncementInvalid
		case *ast.Link:
			if !safeAnnouncementURL(string(v.Destination)) {
				return ast.WalkStop, ErrAnnouncementInvalid
			}
		case *ast.AutoLink:
			if !safeAnnouncementURL(string(v.URL(source))) {
				return ast.WalkStop, ErrAnnouncementInvalid
			}
		case *ast.Heading:
			if v.Level < 2 || v.Level > 4 {
				return ast.WalkStop, ErrAnnouncementInvalid
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return RenderedAnnouncement{}, err
	}
	var rendered bytes.Buffer
	if err = md.Renderer().Render(&rendered, source, doc); err != nil {
		return RenderedAnnouncement{}, ErrAnnouncementInvalid
	}
	policy := bluemonday.NewPolicy()
	policy.AllowElements("p", "br", "hr", "h2", "h3", "h4", "strong", "em", "blockquote", "ul", "ol", "li", "pre", "code", "a")
	policy.AllowAttrs("href").OnElements("a")
	policy.RequireParseableURLs(true).AllowRelativeURLs(false).AllowURLSchemes("https")
	policy.RequireNoFollowOnLinks(true).RequireNoReferrerOnLinks(true).AddTargetBlankToFullyQualifiedLinks(true)
	canonical := policy.Sanitize(rendered.String())
	return RenderedAnnouncement{HTML: canonical, MarkdownHash: fmt.Sprintf("%x", sha256.Sum256(source)), HTMLHash: fmt.Sprintf("%x", sha256.Sum256([]byte(canonical))), PolicyVersion: "announcement-sanitize-v1"}, nil
}

func safeAnnouncementURL(raw string) bool {
	raw = html.UnescapeString(raw)
	if len(raw) > 2048 || strings.ContainsAny(raw, "\\\r\n\t ") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Opaque == ""
}
