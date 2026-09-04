package confluence

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientValidateSpaceAndRoot(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer test-pat" {
			t.Errorf("Authorization = %q", got)
		}
		paths = append(paths, request.URL.RequestURI())
		switch request.URL.Path {
		case "/confluence/rest/api/space/MQMS":
			writeTestJSON(response, `{"key":"MQMS"}`)
		case "/confluence/rest/api/content/123":
			writeTestJSON(response, testPageJSON("123", "MQMS", "Root"))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	config := testClientConfig(server.URL + "/confluence")
	config.RootPage = "123"
	if err := NewClient(server.Client()).Validate(context.Background(), config, "test-pat"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(paths) != 2 || paths[0] != "/confluence/rest/api/space/MQMS" || paths[1] != "/confluence/rest/api/content/123?expand=space%2Cversion" {
		t.Fatalf("requests = %#v", paths)
	}
}

func TestClientValidateMapsSpaceAndRootFailures(t *testing.T) {
	tests := []struct {
		name      string
		root      string
		handler   http.HandlerFunc
		wantError error
	}{
		{
			name: "space missing",
			handler: func(response http.ResponseWriter, _ *http.Request) {
				http.Error(response, "sensitive body", http.StatusNotFound)
			},
			wantError: ErrSpaceNotFound,
		},
		{
			name: "root missing",
			root: "123",
			handler: func(response http.ResponseWriter, request *http.Request) {
				if strings.Contains(request.URL.Path, "/space/") {
					writeTestJSON(response, `{"key":"MQMS"}`)
					return
				}
				http.Error(response, "sensitive body", http.StatusNotFound)
			},
			wantError: ErrRootPageNotFound,
		},
		{
			name: "root wrong space",
			root: "123",
			handler: func(response http.ResponseWriter, request *http.Request) {
				if strings.Contains(request.URL.Path, "/space/") {
					writeTestJSON(response, `{"key":"MQMS"}`)
					return
				}
				writeTestJSON(response, testPageJSON("123", "OTHER", "Root"))
			},
			wantError: ErrRootPageNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			config := testClientConfig(server.URL)
			config.RootPage = test.root
			err := NewClient(server.Client()).Validate(context.Background(), config, "top-secret")
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Validate() error = %v, want %v", err, test.wantError)
			}
			if strings.Contains(err.Error(), "top-secret") || strings.Contains(err.Error(), "sensitive body") {
				t.Fatalf("Validate() error leaked secret data: %q", err)
			}
		})
	}
}

func TestClientDiscoverPaginatesMetadataOnly(t *testing.T) {
	var starts []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/confluence/rest/api/content" {
			http.NotFound(response, request)
			return
		}
		query := request.URL.Query()
		if query.Get("expand") != "space,version" || query.Get("spaceKey") != "MQMS" || query.Get("type") != "page" || query.Get("status") != "current" || query.Get("limit") != "100" {
			t.Errorf("query = %v", query)
		}
		if strings.Contains(query.Get("expand"), "body.storage") {
			t.Errorf("discovery requested body: %v", query)
		}
		starts = append(starts, query.Get("start"))
		if query.Get("start") == "0" {
			writeTestJSON(response, `{"results":[`+testPageJSON("1", "MQMS", "One")+`],"start":0,"limit":100,"size":1,"_links":{"next":"next"}}`)
			return
		}
		writeTestJSON(response, `{"results":[`+testPageJSON("2", "MQMS", "Two")+`],"start":1,"limit":100,"size":1,"_links":{}}`)
	}))
	defer server.Close()

	pages, err := NewClient(server.Client()).Discover(context.Background(), testClientConfig(server.URL+"/confluence"), "test-pat")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(pages) != 2 || pages[0].ID != "1" || pages[1].ID != "2" {
		t.Fatalf("Discover() = %#v", pages)
	}
	if pages[0].URL != server.URL+"/confluence/pages/viewpage.action?pageId=1" {
		t.Errorf("page URL = %q", pages[0].URL)
	}
	if fmt.Sprint(starts) != "[0 1]" {
		t.Errorf("starts = %v", starts)
	}
}

func TestClientDiscoverIncludesRootAndDescendants(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/rest/api/content/10":
			writeTestJSON(response, testPageJSON("10", "MQMS", "Root"))
		case "/rest/api/content/10/descendant/page":
			if request.URL.Query().Get("expand") != "space,version" {
				t.Errorf("query = %v", request.URL.Query())
			}
			writeTestJSON(response, `{"results":[`+testPageJSON("11", "MQMS", "Child")+`],"size":1,"_links":{}}`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	config := testClientConfig(server.URL)
	config.RootPage = "10"
	pages, err := NewClient(server.Client()).Discover(context.Background(), config, "test-pat")
	if err != nil || len(pages) != 2 || pages[0].ID != "10" || pages[1].ID != "11" {
		t.Fatalf("Discover() = %#v, %v", pages, err)
	}
}

func TestClientFetchStorageBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("expand") != "body.storage,space,version" {
			t.Errorf("expand = %q", request.URL.Query().Get("expand"))
		}
		writeTestJSON(response, `{"id":"123","type":"page","title":"Architecture","space":{"key":"MQMS"},"version":{"when":"2026-09-04"},"body":{"storage":{"value":"<h1>Body</h1>"}},"_links":{"webui":"/pages/viewpage.action?pageId=123"}}`)
	}))
	defer server.Close()
	content, err := NewClient(server.Client()).Fetch(context.Background(), testClientConfig(server.URL), "test-pat", "123")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if content.Page.ID != "123" || content.Page.UpdatedAt != "2026-09-04" || content.StorageHTML != "<h1>Body</h1>" {
		t.Fatalf("Fetch() = %#v", content)
	}
}

func TestClientSanitizesRemoteFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				http.Error(response, "response-secret", status)
			}))
			defer server.Close()
			_, err := NewClient(server.Client()).Fetch(context.Background(), testClientConfig(server.URL), "pat-secret", "123")
			if err == nil || strings.Contains(err.Error(), "pat-secret") || strings.Contains(err.Error(), "response-secret") {
				t.Fatalf("Fetch() error = %q", err)
			}
			want := map[int]error{401: ErrAuthentication, 403: ErrForbidden, 404: ErrNotFound, 500: ErrRemote}[status]
			if !errors.Is(err, want) {
				t.Fatalf("Fetch() error = %v, want %v", err, want)
			}
		})
	}
}

func TestProductionClientRejectsRedirectsBeforeSendingAnotherRequest(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		redirected.Store(true)
		writeTestJSON(response, `{"key":"MQMS"}`)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL, http.StatusFound)
	}))
	defer server.Close()

	err := NewClient(nil).Validate(context.Background(), testClientConfig(server.URL), "pat-secret")
	if !errors.Is(err, ErrRemote) {
		t.Fatalf("Validate() error = %v, want ErrRemote", err)
	}
	if redirected.Load() {
		t.Fatal("production client followed an authenticated redirect")
	}
}

func TestClientRejectsMalformedAndOversizedResponses(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			io.WriteString(response, "{")
		}))
		defer server.Close()
		err := NewClient(server.Client()).Validate(context.Background(), testClientConfig(server.URL), "pat")
		if !errors.Is(err, ErrRemote) {
			t.Fatalf("Validate() error = %v, want ErrRemote", err)
		}
	})
	t.Run("oversized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.Header().Set("Content-Type", "application/json")
			io.WriteString(response, `{"value":"`)
			io.WriteString(response, strings.Repeat("x", maxResponseBytes))
			io.WriteString(response, `"}`)
		}))
		defer server.Close()
		err := NewClient(server.Client()).Validate(context.Background(), testClientConfig(server.URL), "pat")
		if !errors.Is(err, ErrRemote) || !strings.Contains(err.Error(), "16 MiB") {
			t.Fatalf("Validate() error = %v, want oversized ErrRemote", err)
		}
	})
}

func TestClientPreservesContextCancellationWithoutTransportLeak(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewClient(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport included pat-secret")
	}))
	err := client.Validate(ctx, testClientConfig("https://wiki.example"), "pat-secret")
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "pat-secret") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestClientRejectsNonProgressingAndRepeatedPagination(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "zero progress", body: `{"results":[],"start":0,"size":0,"_links":{"next":"next"}}`},
		{name: "repeated page", body: `{"results":[],"start":0,"size":1,"_links":{"next":"next"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				writeTestJSON(response, test.body)
			}))
			defer server.Close()
			_, err := NewClient(server.Client()).Discover(context.Background(), testClientConfig(server.URL), "pat")
			if !errors.Is(err, ErrRemote) {
				t.Fatalf("Discover() error = %v, want ErrRemote", err)
			}
		})
	}
}

func TestClientRejectsMalformedPageIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeTestJSON(response, `{"results":[{"id":"1","type":"attachment","title":"Wrong","space":{"key":"MQMS"},"_links":{"webui":"/wrong"}}],"start":0,"size":1,"_links":{}}`)
	}))
	defer server.Close()
	_, err := NewClient(server.Client()).Discover(context.Background(), testClientConfig(server.URL), "pat")
	if !errors.Is(err, ErrRemote) {
		t.Fatalf("Discover() error = %v, want ErrRemote", err)
	}
}

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testClientConfig(baseURL string) Config {
	return Config{BaseURL: baseURL, Space: "MQMS", AuthType: DefaultAuthType}
}

func testPageJSON(id, space, title string) string {
	return fmt.Sprintf(`{"id":%q,"type":"page","title":%q,"space":{"key":%q},"version":{"when":"2026-09-04"},"_links":{"webui":"/pages/viewpage.action?pageId=%s"}}`, id, title, space, id)
}

func writeTestJSON(response http.ResponseWriter, body string) {
	response.Header().Set("Content-Type", "application/json")
	io.WriteString(response, body)
}
