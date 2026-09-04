package confluence

import (
	"strings"
	"testing"
)

func TestFilenameIsUnicodeAwareAndTraversalSafe(t *testing.T) {
	tests := []struct {
		title  string
		pageID string
		want   string
	}{
		{"Architecture Overview", "123", "architecture-overview-123.md"},
		{" Café / 東京 \\ .. Guide ", "456", "café-東京-guide-456.md"},
		{"../../..", "789", "page-789.md"},
		{"Hello...world", "10", "hello-world-10.md"},
	}
	for _, test := range tests {
		got, err := Filename(test.title, test.pageID)
		if err != nil {
			t.Fatalf("Filename(%q) error = %v", test.title, err)
		}
		if got != test.want {
			t.Errorf("Filename(%q) = %q, want %q", test.title, got, test.want)
		}
		if strings.Contains(got, "/") || strings.Contains(got, "\\") || strings.Contains(got, "..") {
			t.Errorf("Filename(%q) = unsafe %q", test.title, got)
		}
	}
	if _, err := Filename("Page", "12x"); err == nil {
		t.Fatal("Filename(invalid ID) error = nil")
	}
}

func TestRenderMarkdownFrontMatterAndStorageElements(t *testing.T) {
	content := PageContent{
		Page: Page{
			ID:        "123",
			Space:     "MQMS",
			Title:     "Architecture: \"Overview\"",
			URL:       "https://wiki.example/pages/viewpage.action?pageId=123",
			UpdatedAt: "2026-09-04T00:00:00.000Z",
		},
		StorageHTML: `<h1>Architecture &amp; Design</h1>
<p>A <strong>bold</strong> paragraph<br>with a <a href="https://example.test">link</a> and <code>x &lt; y</code>.</p>
<ul><li>One</li><li>Two<ol><li>Nested</li></ol></li></ul>
<pre>line 1
line 2</pre>
<table><tbody><tr><th>Name</th><th>Value</th></tr><tr><td>A</td><td>B</td></tr></tbody></table>`,
	}
	markdown, err := RenderMarkdown("resource-1", content)
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	text := string(markdown)
	frontMatter := "---\n" +
		"devdash_resource_id: \"resource-1\"\n" +
		"confluence_page_id: \"123\"\n" +
		"confluence_space: \"MQMS\"\n" +
		"source_url: \"https://wiki.example/pages/viewpage.action?pageId=123\"\n" +
		"title: \"Architecture: \\\"Overview\\\"\"\n" +
		"confluence_updated_at: \"2026-09-04T00:00:00.000Z\"\n---\n\n"
	if !strings.HasPrefix(text, frontMatter) {
		t.Fatalf("front matter =\n%s\nwant prefix\n%s", text, frontMatter)
	}
	for _, want := range []string{
		"# Architecture & Design",
		"A **bold** paragraph  \nwith a [link](https://example.test) and `x < y`.",
		"- One",
		"1. Nested",
		"```\nline 1\nline 2\n```",
		"| Name | Value |\n| --- | --- |\n| A | B |",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Markdown missing %q:\n%s", want, text)
		}
	}
}

func TestRenderMarkdownConvertsCodeMacroAndOptionalTimestamp(t *testing.T) {
	content := PageContent{
		Page:        Page{ID: "7", Space: "DOC", Title: "Code", URL: "https://wiki.example/7"},
		StorageHTML: `<ac:structured-macro ac:name="code"><ac:plain-text-body><![CDATA[fmt.Println("safe")]]></ac:plain-text-body></ac:structured-macro><custom>Readable text</custom>`,
	}
	markdown, err := RenderMarkdown("resource-7", content)
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	text := string(markdown)
	if strings.Contains(text, "confluence_updated_at") {
		t.Errorf("Markdown unexpectedly contains timestamp: %s", text)
	}
	if !strings.Contains(text, "```\nfmt.Println(\"safe\")\n```") || !strings.Contains(text, "Readable text") {
		t.Fatalf("Markdown = %s", text)
	}
}

func TestRenderMarkdownPreservesBreaksAndAvoidsBacktickCollisions(t *testing.T) {
	content := PageContent{
		Page: Page{ID: "8", Space: "DOC", Title: "Formatting", URL: "https://wiki.example/8"},
		StorageHTML: "<p>before<br>after and <code>use `` here</code></p>\n" +
			"<pre>first\n```\nlast</pre>\n" +
			"<ac:structured-macro ac:name=\"code\"><ac:plain-text-body><![CDATA[alpha\n````\nomega]]></ac:plain-text-body></ac:structured-macro>",
	}
	markdown, err := RenderMarkdown("resource-8", content)
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	text := string(markdown)
	for _, want := range []string{
		"before  \nafter and ```use `` here```",
		"````\nfirst\n```\nlast\n````",
		"`````\nalpha\n````\nomega\n`````",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Markdown missing %q:\n%s", want, text)
		}
	}
}
