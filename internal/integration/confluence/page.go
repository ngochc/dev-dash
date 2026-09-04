package confluence

// Page is metadata for one Confluence page.
type Page struct {
	ID        string
	Space     string
	Title     string
	URL       string
	UpdatedAt string
}

// PageContent contains one page and its Confluence storage-format body.
type PageContent struct {
	Page        Page
	StorageHTML string
}
