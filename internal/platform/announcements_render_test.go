package platform

import (
	"strings"
	"testing"
)

func TestAnnouncementRenderer(t *testing.T) {
	out, err := RenderAnnouncement("## News\n\n**Safe** [source](https://example.com/news)\n\n- one\n- two\n\n```go\n<script>\n```")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<h2>News</h2>", "<strong>Safe</strong>", "href=\"https://example.com/news\"", "noopener", "noreferrer", "nofollow", "&lt;script&gt;"} {
		if !strings.Contains(out.HTML, want) {
			t.Errorf("missing %q in %s", want, out.HTML)
		}
	}
	if out.PolicyVersion != "announcement-sanitize-v1" || len(out.MarkdownHash) != 64 || len(out.HTMLHash) != 64 {
		t.Error("missing canonical provenance")
	}
	for _, bad := range []string{"<script>alert(1)</script>", "<img src=x onerror=alert(1)>", "![x](https://example.com/x.png)", "[x](javascript:alert%281%29)", "[x](data:text/html,x)", "[x](//example.com)", "[x](https://user:secret@example.com)", "[x](http://example.com)", "<iframe>bad</iframe>"} {
		if _, err := RenderAnnouncement(bad); err == nil {
			t.Errorf("unsafe markdown accepted: %s", bad)
		}
	}
}

func TestAnnouncementAcknowledgementConsentAndMedia(t *testing.T) {
	base := AnnouncementContent{Title: "Synthetic acknowledgement", Type: "ACKNOWLEDGEMENTS", Visibility: "PUBLIC", Markdown: "Test", Acknowledgements: []Acknowledgement{{DisplayName: "Synthetic fixture", ConsentAttested: true}}}
	if _, err := validAnnouncementContent(&base); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Acknowledgement){func(a *Acknowledgement) { a.ConsentAttested = false }, func(a *Acknowledgement) { a.MediaID = "unverified" }, func(a *Acknowledgement) { a.ExternalLink = "javascript:alert(1)" }, func(a *Acknowledgement) { a.Anonymous = true }} {
		copy := base
		copy.Acknowledgements = append([]Acknowledgement{}, base.Acknowledgements...)
		mutate(&copy.Acknowledgements[0])
		if _, err := validAnnouncementContent(&copy); err == nil {
			t.Fatal("unsafe acknowledgement accepted")
		}
	}
}
