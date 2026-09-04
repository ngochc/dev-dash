package confluence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 16 << 20

var (
	ErrAuthentication   = errors.New("confluence authentication failed")
	ErrForbidden        = errors.New("confluence access forbidden")
	ErrNotFound         = errors.New("confluence resource not found")
	ErrRemote           = errors.New("confluence remote request failed")
	ErrSpaceNotFound    = errors.New("confluence space not found")
	ErrRootPageNotFound = errors.New("confluence root page not found")
)

// HTTPDoer executes HTTP requests.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client accesses the Confluence Data Center REST v1 API.
type Client struct {
	http HTTPDoer
}

func NewClient(doer HTTPDoer) *Client {
	if doer == nil {
		doer = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Client{http: doer}
}

func (c *Client) Validate(ctx context.Context, config Config, pat string) error {
	var space restSpace
	if err := c.getJSON(ctx, config, pat, "/space/"+url.PathEscape(config.Space), nil, &space); err != nil {
		if errors.Is(err, ErrNotFound) {
			return clientError{message: fmt.Sprintf("Confluence space %q was not found", config.Space), cause: ErrSpaceNotFound}
		}
		return err
	}
	if strings.TrimSpace(space.Key) == "" || space.Key != config.Space {
		return clientError{message: "Confluence returned an invalid space identity", cause: ErrRemote}
	}
	if config.RootPage == "" {
		return nil
	}
	page, err := c.fetchPageMetadata(ctx, config, pat, config.RootPage)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return clientError{message: fmt.Sprintf("Confluence root page %q was not found", config.RootPage), cause: ErrRootPageNotFound}
		}
		return err
	}
	if page.Space != config.Space {
		return clientError{message: fmt.Sprintf("Confluence root page %q is not in space %q", config.RootPage, config.Space), cause: ErrRootPageNotFound}
	}
	return nil
}

func (c *Client) Discover(ctx context.Context, config Config, pat string) ([]Page, error) {
	pages := make([]Page, 0)
	path := "/content"
	values := url.Values{
		"spaceKey": {config.Space},
		"type":     {"page"},
		"status":   {"current"},
		"expand":   {"space,version"},
		"limit":    {"100"},
	}
	if config.RootPage != "" {
		root, err := c.fetchPageMetadata(ctx, config, pat, config.RootPage)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, clientError{message: fmt.Sprintf("Confluence root page %q was not found", config.RootPage), cause: ErrRootPageNotFound}
			}
			return nil, err
		}
		if root.Space != config.Space {
			return nil, clientError{message: fmt.Sprintf("Confluence root page %q is not in space %q", config.RootPage, config.Space), cause: ErrRootPageNotFound}
		}
		pages = append(pages, root)
		path = "/content/" + url.PathEscape(config.RootPage) + "/descendant/page"
		values = url.Values{
			"expand": {"space,version"},
			"limit":  {"100"},
		}
	}

	start := 0
	seenStarts := make(map[int]struct{})
	for {
		if _, repeated := seenStarts[start]; repeated {
			return nil, clientError{message: "Confluence pagination repeated a page", cause: ErrRemote}
		}
		seenStarts[start] = struct{}{}
		values.Set("start", fmt.Sprintf("%d", start))
		var response restPageList
		if err := c.getJSON(ctx, config, pat, path, values, &response); err != nil {
			return nil, err
		}
		if response.Start != start {
			return nil, clientError{message: "Confluence pagination repeated a page", cause: ErrRemote}
		}
		for _, item := range response.Results {
			page, err := pageFromREST(config, item)
			if err != nil {
				return nil, err
			}
			if page.Space != config.Space {
				return nil, clientError{message: "Confluence returned a page from an unexpected space", cause: ErrRemote}
			}
			pages = append(pages, page)
		}
		if response.Links.Next == "" {
			break
		}
		if response.Size <= 0 {
			return nil, clientError{message: "Confluence pagination made no progress", cause: ErrRemote}
		}
		start += response.Size
	}
	return pages, nil
}

func (c *Client) Fetch(ctx context.Context, config Config, pat, pageID string) (PageContent, error) {
	if !pageIDPattern.MatchString(pageID) {
		return PageContent{}, clientError{message: "Invalid Confluence page identity", cause: ErrRemote}
	}
	values := url.Values{"expand": {"body.storage,space,version"}}
	var response restPage
	if err := c.getJSON(ctx, config, pat, "/content/"+url.PathEscape(pageID), values, &response); err != nil {
		return PageContent{}, err
	}
	page, err := pageFromREST(config, response)
	if err != nil {
		return PageContent{}, err
	}
	if page.ID != pageID || page.Space != config.Space {
		return PageContent{}, clientError{message: "Confluence returned an unexpected page identity", cause: ErrRemote}
	}
	if response.Body.Storage == nil {
		return PageContent{}, clientError{message: "Confluence returned no storage body", cause: ErrRemote}
	}
	return PageContent{Page: page, StorageHTML: response.Body.Storage.Value}, nil
}

func (c *Client) fetchPageMetadata(ctx context.Context, config Config, pat, pageID string) (Page, error) {
	if !pageIDPattern.MatchString(pageID) {
		return Page{}, clientError{message: "Invalid Confluence page identity", cause: ErrRemote}
	}
	values := url.Values{"expand": {"space,version"}}
	var response restPage
	if err := c.getJSON(ctx, config, pat, "/content/"+url.PathEscape(pageID), values, &response); err != nil {
		return Page{}, err
	}
	page, err := pageFromREST(config, response)
	if err != nil {
		return Page{}, err
	}
	if page.ID != pageID {
		return Page{}, clientError{message: "Confluence returned an unexpected page identity", cause: ErrRemote}
	}
	return page, nil
}

func (c *Client) getJSON(ctx context.Context, config Config, pat, path string, query url.Values, destination any) error {
	endpoint := config.BaseURL + "/rest/api" + path
	if len(query) != 0 {
		endpoint += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return clientError{message: "Build Confluence request failed", cause: ErrRemote}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+pat)
	response, err := c.http.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return clientError{message: "Confluence request failed", cause: ErrRemote}
	}
	if response == nil || response.Body == nil {
		return clientError{message: "Confluence returned an invalid response", cause: ErrRemote}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return statusError(response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return clientError{message: "Read Confluence response failed", cause: ErrRemote}
	}
	if len(body) > maxResponseBytes {
		return clientError{message: "Confluence response exceeded 16 MiB", cause: ErrRemote}
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return clientError{message: "Confluence returned malformed JSON", cause: ErrRemote}
	}
	return nil
}

func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return clientError{message: "Confluence authentication failed", cause: ErrAuthentication}
	case http.StatusForbidden:
		return clientError{message: "Confluence access forbidden", cause: ErrForbidden}
	case http.StatusNotFound:
		return clientError{message: "Confluence resource not found", cause: ErrNotFound}
	default:
		return clientError{message: fmt.Sprintf("Confluence request failed with status %d", status), cause: ErrRemote}
	}
}

func pageFromREST(config Config, value restPage) (Page, error) {
	if value.Type != "page" || !pageIDPattern.MatchString(value.ID) || strings.TrimSpace(value.Title) == "" || strings.TrimSpace(value.Space.Key) == "" || strings.TrimSpace(value.Links.WebUI) == "" {
		return Page{}, clientError{message: "Confluence returned an invalid page identity", cause: ErrRemote}
	}
	pageURL, err := resolveWebUI(config.BaseURL, value.Links.WebUI)
	if err != nil {
		return Page{}, clientError{message: "Confluence returned an invalid page URL", cause: ErrRemote}
	}
	return Page{ID: value.ID, Space: value.Space.Key, Title: value.Title, URL: pageURL, UpdatedAt: value.Version.When}, nil
}

func resolveWebUI(baseURL, webUI string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	reference, err := url.Parse(webUI)
	if err != nil || reference.Host != "" || reference.Scheme != "" {
		return "", errors.New("invalid web UI reference")
	}
	contextPath := strings.TrimRight(base.Path, "/")
	path := reference.Path
	if contextPath != "" && path != contextPath && !strings.HasPrefix(path, contextPath+"/") {
		path = contextPath + "/" + strings.TrimLeft(path, "/")
	}
	if path == "" {
		return "", errors.New("empty web UI path")
	}
	base.Path = path
	base.RawPath = ""
	base.RawQuery = reference.RawQuery
	base.Fragment = reference.Fragment
	return base.String(), nil
}

type clientError struct {
	message string
	cause   error
}

func (e clientError) Error() string { return e.message }
func (e clientError) Unwrap() error { return e.cause }

type restSpace struct {
	Key string `json:"key"`
}

type restPage struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	Space struct {
		Key string `json:"key"`
	} `json:"space"`
	Version struct {
		When string `json:"when"`
	} `json:"version"`
	Links struct {
		WebUI string `json:"webui"`
	} `json:"_links"`
	Body struct {
		Storage *struct {
			Value string `json:"value"`
		} `json:"storage"`
	} `json:"body"`
}

type restPageList struct {
	Results []restPage `json:"results"`
	Start   int        `json:"start"`
	Limit   int        `json:"limit"`
	Size    int        `json:"size"`
	Links   struct {
		Next string `json:"next"`
	} `json:"_links"`
}
