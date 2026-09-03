package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ngochc/dev-dash/internal/resource"
)

func TestMembershipServiceAddResolvesWorkspaceAndPreservesRole(t *testing.T) {
	workspaces := newFakeRepository(Workspace{ID: "workspace-id", Name: "devdash"})
	resources := newMembershipResourceRepository(resource.Resource{ID: "resource-id", Name: "api"})
	memberships := newFakeMembershipRepository()
	service := NewMembershipService(workspaces, resources, memberships)

	item, err := service.Add(context.Background(), "devdash", "resource-id", " custom role ")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if item.ID != "workspace-id" {
		t.Errorf("Add() workspace = %#v, want name resolution", item)
	}
	stored := memberships.items[membershipKey("workspace-id", "resource-id")]
	if stored.Role != " custom role " {
		t.Errorf("Add() role = %q, want caller text preserved", stored.Role)
	}
}

func TestMembershipServiceUsesWorkspaceIDBeforeName(t *testing.T) {
	workspaces := newFakeRepository(
		Workspace{ID: "target", Name: "by-id"},
		Workspace{ID: "other", Name: "target"},
	)
	resources := newMembershipResourceRepository(resource.Resource{ID: "resource-id"})
	memberships := newFakeMembershipRepository()
	item, err := NewMembershipService(workspaces, resources, memberships).Add(context.Background(), "target", "resource-id", "")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if item.Name != "by-id" {
		t.Errorf("Add() workspace = %#v, want ID match", item)
	}
}

func TestMembershipServiceValidatesResourceAfterWorkspaceResolution(t *testing.T) {
	workspaces := newFakeRepository()
	resources := newMembershipResourceRepository()
	memberships := newFakeMembershipRepository()
	service := NewMembershipService(workspaces, resources, memberships)

	if _, err := service.Add(context.Background(), "missing", "", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Add(missing workspace) error = %v, want workspace ErrNotFound", err)
	}
	workspaces.items = append(workspaces.items, Workspace{ID: "workspace-id", Name: "devdash"})
	if _, err := service.Add(context.Background(), "devdash", " ", ""); err == nil || err.Error() != "resource ID is required" {
		t.Fatalf("Add(blank resource) error = %v, want required ID", err)
	}
	if resources.getCalls != 0 || memberships.addCalls != 0 {
		t.Errorf("repository calls = resource %d membership %d, want zero", resources.getCalls, memberships.addCalls)
	}
}

func TestMembershipServiceUnknownAndDuplicateResourceErrors(t *testing.T) {
	workspaces := newFakeRepository(Workspace{ID: "workspace-id", Name: "devdash"})
	resources := newMembershipResourceRepository(resource.Resource{ID: "resource-id"})
	memberships := newFakeMembershipRepository()
	service := NewMembershipService(workspaces, resources, memberships)

	_, err := service.Add(context.Background(), "devdash", "missing", "")
	if err == nil || err.Error() != `resource "missing" not found` || !errors.Is(err, resource.ErrNotFound) {
		t.Fatalf("Add(missing) error = %v, want exact resource ErrNotFound", err)
	}
	if _, err := service.Add(context.Background(), "devdash", "resource-id", "primary"); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	_, err = service.Add(context.Background(), "devdash", "resource-id", "dependency")
	if err == nil || err.Error() != `resource "resource-id" is already in workspace "devdash"` || !errors.Is(err, ErrMembershipExists) {
		t.Fatalf("duplicate Add() error = %v, want exact ErrMembershipExists", err)
	}
	if got := memberships.items[membershipKey("workspace-id", "resource-id")].Role; got != "primary" {
		t.Errorf("duplicate Add() role = %q, want original primary", got)
	}
}

func TestMembershipServiceListSortsByResourceNameThenID(t *testing.T) {
	workspaces := newFakeRepository(Workspace{ID: "workspace-id", Name: "devdash"})
	resources := newMembershipResourceRepository()
	memberships := newFakeMembershipRepository(
		ResourceMembership{WorkspaceID: "workspace-id", ResourceID: "2", Resource: resource.Resource{ID: "2", Name: "beta"}},
		ResourceMembership{WorkspaceID: "workspace-id", ResourceID: "3", Resource: resource.Resource{ID: "3", Name: "alpha"}},
		ResourceMembership{WorkspaceID: "workspace-id", ResourceID: "1", Resource: resource.Resource{ID: "1", Name: "alpha"}},
	)
	workspaceItem, items, err := NewMembershipService(workspaces, resources, memberships).List(context.Background(), "devdash")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if workspaceItem.ID != "workspace-id" {
		t.Errorf("List() workspace = %#v", workspaceItem)
	}
	got := []string{items[0].Resource.ID, items[1].Resource.ID, items[2].Resource.ID}
	if want := []string{"1", "3", "2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("List() IDs = %v, want %v", got, want)
	}
}

func TestMembershipServiceRemoveDoesNotDeleteResource(t *testing.T) {
	workspaces := newFakeRepository(Workspace{ID: "workspace-id", Name: "devdash"})
	resources := newMembershipResourceRepository(resource.Resource{ID: "resource-id"})
	memberships := newFakeMembershipRepository(ResourceMembership{WorkspaceID: "workspace-id", ResourceID: "resource-id"})
	service := NewMembershipService(workspaces, resources, memberships)

	item, err := service.Remove(context.Background(), "devdash", "resource-id")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if item.ID != "workspace-id" || len(memberships.items) != 0 {
		t.Errorf("Remove() workspace = %#v, memberships = %#v", item, memberships.items)
	}
	if _, err := resources.Get(context.Background(), "resource-id"); err != nil {
		t.Errorf("resource after Remove() error = %v", err)
	}
	_, err = service.Remove(context.Background(), "devdash", "resource-id")
	if err == nil || err.Error() != `resource "resource-id" is not in workspace "devdash"` || !errors.Is(err, ErrMembershipNotFound) {
		t.Fatalf("missing Remove() error = %v, want exact ErrMembershipNotFound", err)
	}
}

type membershipResourceRepository struct {
	items    map[string]resource.Resource
	getCalls int
}

func newMembershipResourceRepository(items ...resource.Resource) *membershipResourceRepository {
	r := &membershipResourceRepository{items: make(map[string]resource.Resource)}
	for _, item := range items {
		r.items[item.ID] = item
	}
	return r
}
func (r *membershipResourceRepository) Create(_ context.Context, item resource.Resource) error {
	r.items[item.ID] = item
	return nil
}
func (r *membershipResourceRepository) List(context.Context) ([]resource.Resource, error) {
	return nil, nil
}
func (r *membershipResourceRepository) Get(_ context.Context, id string) (resource.Resource, error) {
	r.getCalls++
	item, ok := r.items[id]
	if !ok {
		return resource.Resource{}, resource.ErrNotFound
	}
	return item, nil
}
func (r *membershipResourceRepository) Update(_ context.Context, item resource.Resource) error {
	r.items[item.ID] = item
	return nil
}
func (r *membershipResourceRepository) Delete(_ context.Context, id string) error {
	delete(r.items, id)
	return nil
}

type fakeMembershipRepository struct {
	items       map[string]ResourceMembership
	addCalls    int
	removeCalls int
}

func newFakeMembershipRepository(items ...ResourceMembership) *fakeMembershipRepository {
	r := &fakeMembershipRepository{items: make(map[string]ResourceMembership)}
	for _, item := range items {
		r.items[membershipKey(item.WorkspaceID, item.ResourceID)] = item
	}
	return r
}
func (r *fakeMembershipRepository) Add(_ context.Context, item ResourceMembership) error {
	r.addCalls++
	key := membershipKey(item.WorkspaceID, item.ResourceID)
	if _, exists := r.items[key]; exists {
		return ErrMembershipExists
	}
	r.items[key] = item
	return nil
}
func (r *fakeMembershipRepository) ListByWorkspace(_ context.Context, workspaceID string) ([]ResourceMembership, error) {
	var items []ResourceMembership
	for _, item := range r.items {
		if item.WorkspaceID == workspaceID {
			items = append(items, item)
		}
	}
	return items, nil
}
func (r *fakeMembershipRepository) Remove(_ context.Context, workspaceID, resourceID string) error {
	r.removeCalls++
	key := membershipKey(workspaceID, resourceID)
	if _, exists := r.items[key]; !exists {
		return ErrMembershipNotFound
	}
	delete(r.items, key)
	return nil
}
func membershipKey(workspaceID, resourceID string) string { return workspaceID + "\x00" + resourceID }
