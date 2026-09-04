package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ngochc/dev-dash/internal/wiki"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestExecuteWikiRefreshUsesFeedbackStream(t *testing.T) {
	var output, feedback bytes.Buffer
	service := &fakeWikiService{refreshCount: 3}
	service.beforeRefresh = func() {
		if got := feedback.String(); got != "Refreshing wiki pages...\n" {
			t.Errorf("feedback during refresh = %q", got)
		}
	}
	if err := executeWiki(context.Background(), []string{"refresh", "workspace"}, &output, &feedback, service); err != nil {
		t.Fatalf("executeWiki(refresh) error = %v", err)
	}
	if output.String() != "Wiki pages refreshed: 3\n" {
		t.Errorf("output = %q", output.String())
	}
	if feedback.String() != "Refreshing wiki pages...\nRefreshing wiki pages: done\n" {
		t.Errorf("feedback = %q", feedback.String())
	}
}

func TestExecuteWikiListFormatsStates(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var output bytes.Buffer
		if err := executeWiki(context.Background(), []string{"list", "workspace"}, &output, &bytes.Buffer{}, &fakeWikiService{}); err != nil {
			t.Fatal(err)
		}
		if output.String() != "No wiki pages found.\n" {
			t.Fatalf("output = %q", output.String())
		}
	})
	t.Run("pages", func(t *testing.T) {
		service := &fakeWikiService{listed: []wiki.Listed{
			{Page: wiki.Page{PageID: "1", Title: "One", MaterializedPath: "/wiki/one.md"}, State: wiki.StateFetched},
			{Page: wiki.Page{PageID: "2", Title: "Two"}, State: wiki.StateNotFetched},
		}}
		var output bytes.Buffer
		if err := executeWiki(context.Background(), []string{"list", "workspace"}, &output, &bytes.Buffer{}, service); err != nil {
			t.Fatal(err)
		}
		want := []string{"PAGE", "ID", "STATUS", "TITLE", "PATH", "1", "fetched", "One", "/wiki/one.md", "2", "not-fetched", "Two", "-"}
		if got := strings.Fields(output.String()); !reflect.DeepEqual(got, want) {
			t.Fatalf("fields = %v, want %v", got, want)
		}
	})
}

func TestExecuteWikiFetchPrintsResultsBeforeAggregateError(t *testing.T) {
	fetchErr := errors.New("wiki fetch failed for 1 page(s)")
	service := &fakeWikiService{
		fetchErr: fetchErr,
		fetchResults: []wiki.FetchResult{
			{PageID: "1", Status: "fetched", Path: "/wiki/one.md"},
			{PageID: "2", Path: "/wiki/two.md", Error: errors.New("destination conflict")},
		},
	}
	var output, feedback bytes.Buffer
	err := executeWiki(context.Background(), []string{"fetch", "workspace", "2", "1"}, &output, &feedback, service)
	if !errors.Is(err, fetchErr) {
		t.Fatalf("executeWiki(fetch) error = %v", err)
	}
	want := []string{"PAGE", "ID", "STATUS", "PATH", "1", "fetched", "/wiki/one.md", "2", "failed:", "destination", "conflict", "/wiki/two.md"}
	if got := strings.Fields(output.String()); !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
	if strings.Contains(output.String(), "Refreshing") {
		t.Fatalf("result output contains progress: %q", output.String())
	}
	if feedback.String() != "Refreshing and fetching wiki pages...\nRefreshing and fetching wiki pages: failed\n" {
		t.Fatalf("feedback = %q", feedback.String())
	}
	if !reflect.DeepEqual(service.selected, []string{"2", "1"}) || service.fetchAllCalls != 0 {
		t.Fatalf("selection = %v all calls = %d", service.selected, service.fetchAllCalls)
	}
}

func TestExecuteWikiFetchAllUsesDedicatedMethod(t *testing.T) {
	service := &fakeWikiService{}
	if err := executeWiki(context.Background(), []string{"fetch", "workspace", "--all"}, &bytes.Buffer{}, &bytes.Buffer{}, service); err != nil {
		t.Fatal(err)
	}
	if service.fetchAllCalls != 1 || service.fetchSelectedCalls != 0 {
		t.Fatalf("fetch calls = all %d selected %d", service.fetchAllCalls, service.fetchSelectedCalls)
	}
}

func TestWikiCommandsValidateBeforeDatabaseOpen(t *testing.T) {
	tests := []struct {
		args    []string
		wantErr string
	}{
		{args: []string{"wiki"}, wantErr: "wiki command required: refresh, list, or fetch"},
		{args: []string{"wiki", "unknown"}, wantErr: "unknown wiki command: unknown"},
		{args: []string{"wiki", "refresh"}, wantErr: "usage: devdash wiki refresh <workspace>"},
		{args: []string{"wiki", "refresh", "ws", "extra"}, wantErr: "usage: devdash wiki refresh <workspace>"},
		{args: []string{"wiki", "list"}, wantErr: "usage: devdash wiki list <workspace>"},
		{args: []string{"wiki", "list", "ws", "extra"}, wantErr: "usage: devdash wiki list <workspace>"},
		{args: []string{"wiki", "fetch", "ws"}, wantErr: "usage: devdash wiki fetch <workspace> --all|<page> [<page>...]"},
		{args: []string{"wiki", "fetch", "ws", "--all", "1"}, wantErr: "usage: devdash wiki fetch <workspace> --all|<page> [<page>...]"},
		{args: []string{"wiki", "fetch", "ws", "1", "--all"}, wantErr: "usage: devdash wiki fetch <workspace> --all|<page> [<page>...]"},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "devdash.db")
			t.Setenv("DEVDASH_DB", databasePath)
			err := run(context.Background(), test.args, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("run(%v) error = %v, want %q", test.args, err, test.wantErr)
			}
			if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
				t.Fatalf("database stat error = %v, want missing", statErr)
			}
		})
	}
}

type fakeWikiService struct {
	refreshCount       int
	refreshErr         error
	listed             []wiki.Listed
	listErr            error
	fetchResults       []wiki.FetchResult
	fetchErr           error
	selected           []string
	fetchAllCalls      int
	fetchSelectedCalls int
	beforeRefresh      func()
}

func (s *fakeWikiService) Refresh(context.Context, string) (workspace.Workspace, int, error) {
	if s.beforeRefresh != nil {
		s.beforeRefresh()
	}
	return workspace.Workspace{}, s.refreshCount, s.refreshErr
}
func (s *fakeWikiService) List(context.Context, string) (workspace.Workspace, []wiki.Listed, error) {
	return workspace.Workspace{}, append([]wiki.Listed(nil), s.listed...), s.listErr
}
func (s *fakeWikiService) FetchSelected(_ context.Context, _ string, selectors []string) (workspace.Workspace, []wiki.FetchResult, error) {
	s.fetchSelectedCalls++
	s.selected = append([]string(nil), selectors...)
	return workspace.Workspace{}, append([]wiki.FetchResult(nil), s.fetchResults...), s.fetchErr
}
func (s *fakeWikiService) FetchAll(context.Context, string) (workspace.Workspace, []wiki.FetchResult, error) {
	s.fetchAllCalls++
	return workspace.Workspace{}, append([]wiki.FetchResult(nil), s.fetchResults...), s.fetchErr
}
