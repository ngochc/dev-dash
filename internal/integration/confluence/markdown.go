package confluence

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const markdownLineBreak = "\x00devdash-line-break\x00"

// Filename returns a stable, traversal-safe generated filename.
func Filename(title, pageID string) (string, error) {
	if !pageIDPattern.MatchString(pageID) {
		return "", fmt.Errorf("invalid Confluence page ID %q", pageID)
	}
	var slug strings.Builder
	separator := false
	for _, value := range title {
		if unicode.IsLetter(value) || unicode.IsNumber(value) {
			if separator && slug.Len() != 0 {
				slug.WriteByte('-')
			}
			slug.WriteRune(unicode.ToLower(value))
			separator = false
			continue
		}
		separator = true
	}
	if slug.Len() == 0 {
		slug.WriteString("page")
	}
	return slug.String() + "-" + pageID + ".md", nil
}

// RenderMarkdown emits deterministic front matter and converts storage XHTML.
func RenderMarkdown(resourceID string, content PageContent) ([]byte, error) {
	if strings.TrimSpace(resourceID) == "" {
		return nil, fmt.Errorf("devdash resource ID is required")
	}
	if !pageIDPattern.MatchString(content.Page.ID) {
		return nil, fmt.Errorf("invalid Confluence page ID %q", content.Page.ID)
	}
	body, err := storageToMarkdown(content.StorageHTML)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.WriteString("---\n")
	writeFrontMatter(&output, "devdash_resource_id", resourceID)
	writeFrontMatter(&output, "confluence_page_id", content.Page.ID)
	writeFrontMatter(&output, "confluence_space", content.Page.Space)
	writeFrontMatter(&output, "source_url", content.Page.URL)
	writeFrontMatter(&output, "title", content.Page.Title)
	if content.Page.UpdatedAt != "" {
		writeFrontMatter(&output, "confluence_updated_at", content.Page.UpdatedAt)
	}
	output.WriteString("---\n\n")
	output.WriteString(body)
	if body != "" && !strings.HasSuffix(body, "\n") {
		output.WriteByte('\n')
	}
	return output.Bytes(), nil
}

func writeFrontMatter(output *bytes.Buffer, key, value string) {
	fmt.Fprintf(output, "%s: %s\n", key, strconv.Quote(value))
}

func storageToMarkdown(storage string) (string, error) {
	contextNode := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := html.ParseFragment(strings.NewReader(storage), contextNode)
	if err != nil {
		return "", fmt.Errorf("parse Confluence storage XHTML: %w", err)
	}
	renderer := markdownRenderer{}
	for _, node := range nodes {
		renderer.renderBlock(node)
	}
	return strings.Join(renderer.blocks, "\n\n"), nil
}

type markdownRenderer struct {
	blocks []string
}

func (r *markdownRenderer) renderBlock(node *html.Node) {
	if node.Type == html.TextNode {
		r.addBlock(normalizeInline(node.Data))
		return
	}
	if node.Type != html.ElementNode {
		return
	}
	name := strings.ToLower(node.Data)
	switch name {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(name[1] - '0')
		r.addBlock(strings.Repeat("#", level) + " " + renderInlineChildren(node))
	case "p":
		r.addBlock(renderInlineChildren(node))
	case "br":
		r.addBlock("")
	case "ul":
		r.addBlock(renderList(node, false, 0))
	case "ol":
		r.addBlock(renderList(node, true, 0))
	case "pre":
		r.addBlock(renderFencedCode(strings.TrimRight(nodeText(node), "\n")))
	case "table":
		r.addBlock(renderTable(node))
	case "ac:structured-macro", "structured-macro":
		if macroName(node) == "code" {
			r.addBlock(renderFencedCode(strings.TrimSpace(codeMacroText(node))))
			return
		}
		r.renderChildren(node)
	case "div", "section", "article", "body", "html":
		r.renderChildren(node)
	default:
		if hasBlockChild(node) {
			r.renderChildren(node)
			return
		}
		r.addBlock(renderInline(node))
	}
}

func (r *markdownRenderer) renderChildren(node *html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		r.renderBlock(child)
	}
}

func (r *markdownRenderer) addBlock(value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		r.blocks = append(r.blocks, value)
	}
}

func renderInlineChildren(node *html.Node) string {
	var output strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		output.WriteString(renderInline(child))
	}
	return normalizeInline(output.String())
}

func renderInline(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	if node.Type != html.ElementNode {
		return ""
	}
	name := strings.ToLower(node.Data)
	children := func() string {
		var output strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			output.WriteString(renderInline(child))
		}
		return output.String()
	}()
	switch name {
	case "br":
		return markdownLineBreak
	case "strong", "b":
		return "**" + normalizeInline(children) + "**"
	case "em", "i":
		return "*" + normalizeInline(children) + "*"
	case "code":
		return renderInlineCode(strings.TrimSpace(nodeText(node)))
	case "a":
		label := normalizeInline(children)
		href := attribute(node, "href")
		if href == "" {
			return label
		}
		return "[" + label + "](" + href + ")"
	case "img":
		return attribute(node, "alt")
	default:
		return children
	}
}

func renderInlineCode(text string) string {
	marker := backtickDelimiter(text, 1)
	padding := ""
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		padding = " "
	}
	return marker + padding + text + padding + marker
}

func renderFencedCode(text string) string {
	marker := backtickDelimiter(text, 3)
	return marker + "\n" + text + "\n" + marker
}

func backtickDelimiter(text string, minimum int) string {
	longest := 0
	current := 0
	for _, character := range text {
		if character == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	width := minimum
	if longest >= width {
		width = longest + 1
	}
	return strings.Repeat("`", width)
}

func renderList(list *html.Node, ordered bool, depth int) string {
	var lines []string
	index := 1
	for item := list.FirstChild; item != nil; item = item.NextSibling {
		if item.Type != html.ElementNode || strings.ToLower(item.Data) != "li" {
			continue
		}
		var label strings.Builder
		var nested []string
		for child := item.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && (strings.EqualFold(child.Data, "ul") || strings.EqualFold(child.Data, "ol")) {
				nested = append(nested, renderList(child, strings.EqualFold(child.Data, "ol"), depth+1))
				continue
			}
			label.WriteString(renderInline(child))
		}
		marker := "-"
		if ordered {
			marker = fmt.Sprintf("%d.", index)
		}
		lines = append(lines, strings.Repeat("  ", depth)+marker+" "+normalizeInline(label.String()))
		lines = append(lines, nested...)
		index++
	}
	return strings.Join(lines, "\n")
}

func renderTable(table *html.Node) string {
	var rows [][]string
	walkElements(table, "tr", func(row *html.Node) {
		var cells []string
		for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.Type == html.ElementNode && (strings.EqualFold(cell.Data, "th") || strings.EqualFold(cell.Data, "td")) {
				value := strings.ReplaceAll(renderInlineChildren(cell), "|", "\\|")
				cells = append(cells, value)
			}
		}
		if len(cells) != 0 {
			rows = append(rows, cells)
		}
	})
	if len(rows) == 0 {
		return ""
	}
	columns := len(rows[0])
	var lines []string
	lines = append(lines, tableRow(rows[0], columns))
	separator := make([]string, columns)
	for i := range separator {
		separator[i] = "---"
	}
	lines = append(lines, tableRow(separator, columns))
	for _, row := range rows[1:] {
		lines = append(lines, tableRow(row, columns))
	}
	return strings.Join(lines, "\n")
}

func tableRow(cells []string, columns int) string {
	padded := make([]string, columns)
	copy(padded, cells)
	return "| " + strings.Join(padded, " | ") + " |"
}

func walkElements(node *html.Node, name string, visit func(*html.Node)) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && strings.EqualFold(child.Data, name) {
			visit(child)
			continue
		}
		walkElements(child, name, visit)
	}
}

func hasBlockChild(node *html.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		switch strings.ToLower(child.Data) {
		case "h1", "h2", "h3", "h4", "h5", "h6", "p", "div", "ul", "ol", "pre", "table", "ac:structured-macro", "structured-macro":
			return true
		}
	}
	return false
}

func macroName(node *html.Node) string {
	for _, attribute := range node.Attr {
		if (attribute.Namespace == "ac" && attribute.Key == "name") || attribute.Key == "ac:name" || attribute.Key == "name" {
			return strings.ToLower(attribute.Val)
		}
	}
	return ""
}

func codeMacroText(node *html.Node) string {
	var text string
	walkAll(node, func(child *html.Node) {
		if text != "" || child.Type != html.ElementNode {
			return
		}
		if strings.EqualFold(child.Data, "ac:plain-text-body") || strings.EqualFold(child.Data, "plain-text-body") {
			text = nodeTextIncludingComments(child)
		}
	})
	if text == "" {
		text = nodeTextIncludingComments(node)
	}
	return strings.TrimSuffix(strings.TrimPrefix(text, "[CDATA["), "]]")
}

func walkAll(node *html.Node, visit func(*html.Node)) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		visit(child)
		walkAll(child, visit)
	}
}

func nodeText(node *html.Node) string {
	var output strings.Builder
	walkAll(node, func(child *html.Node) {
		if child.Type == html.TextNode {
			output.WriteString(child.Data)
		}
	})
	return output.String()
}

func nodeTextIncludingComments(node *html.Node) string {
	var output strings.Builder
	walkAll(node, func(child *html.Node) {
		if child.Type == html.TextNode || child.Type == html.CommentNode {
			output.WriteString(child.Data)
		}
	})
	return output.String()
}

func attribute(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func normalizeInline(value string) string {
	parts := strings.Split(value, markdownLineBreak)
	for i := range parts {
		parts[i] = strings.Join(strings.Fields(parts[i]), " ")
	}
	return strings.Join(parts, "  \n")
}
