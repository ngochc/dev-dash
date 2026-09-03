package repository

import (
	"context"
	"reflect"
	"testing"
)

func TestDeriveState(t *testing.T) {
	for _, test := range []struct {
		name       string
		path       string
		inspection CheckoutInspection
		want       State
		wantCalls  int
	}{
		{name: "not cloned", want: StateNotCloned},
		{name: "cloned", path: "/repo", inspection: CheckoutInspection{Exists: true, Valid: true}, want: StateCloned, wantCalls: 1},
		{name: "missing", path: "/repo", inspection: CheckoutInspection{}, want: StateMissing, wantCalls: 1},
		{name: "invalid", path: "/repo", inspection: CheckoutInspection{Exists: true}, want: StateInvalid, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			inspector := &fakeInspector{inspection: test.inspection}
			state, err := DeriveState(context.Background(), Repository{CheckoutPath: test.path, URL: "https://github.com/org/repo"}, inspector)
			if err != nil {
				t.Fatalf("DeriveState() error = %v", err)
			}
			if state != test.want {
				t.Errorf("DeriveState() = %q, want %q", state, test.want)
			}
			if inspector.calls != test.wantCalls {
				t.Errorf("inspector calls = %d, want %d", inspector.calls, test.wantCalls)
			}
		})
	}
}

func TestSelectRepositories(t *testing.T) {
	items := []Repository{
		{ResourceID: "1", ExternalKey: "org-a/common", Name: "common"},
		{ResourceID: "2", ExternalKey: "org-b/common", Name: "common"},
		{ResourceID: "3", ExternalKey: "org-a/api", Name: "api"},
	}
	selected, err := Select(items, []string{"api", "org-a/common"}, false)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	want := []Repository{items[2], items[0]}
	if !reflect.DeepEqual(selected, want) {
		t.Errorf("Select() = %#v, want %#v", selected, want)
	}
	if _, err := Select(items, []string{"common"}, false); err == nil || err.Error() != `repository "common" is ambiguous; use owner/repository` {
		t.Errorf("ambiguous Select() error = %v", err)
	}
	if _, err := Select(items, []string{"missing"}, false); err == nil || err.Error() != `repository "missing" not found` {
		t.Errorf("missing Select() error = %v", err)
	}
}

type fakeInspector struct {
	inspection CheckoutInspection
	calls      int
}

func (i *fakeInspector) Inspect(context.Context, string, string) (CheckoutInspection, error) {
	i.calls++
	return i.inspection, nil
}
