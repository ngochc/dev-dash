package workspace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ngochc/dev-dash/internal/resource"
)

var (
	ErrMembershipExists   = errors.New("workspace resource membership already exists")
	ErrMembershipNotFound = errors.New("workspace resource membership not found")
)

type ResourceMembership struct {
	WorkspaceID string
	ResourceID  string
	Role        string
	Resource    resource.Resource
	CreatedAt   time.Time
}

type MembershipRepository interface {
	Add(context.Context, ResourceMembership) error
	ListByWorkspace(context.Context, string) ([]ResourceMembership, error)
	Remove(context.Context, string, string) error
}

type MembershipService struct {
	workspaceService     *Service
	resourceRepository   resource.Repository
	membershipRepository MembershipRepository
}

func NewMembershipService(workspaceRepository Repository, resourceRepository resource.Repository, membershipRepository MembershipRepository) *MembershipService {
	return &MembershipService{
		workspaceService:     NewService(workspaceRepository),
		resourceRepository:   resourceRepository,
		membershipRepository: membershipRepository,
	}
}

func (s *MembershipService) Add(ctx context.Context, workspaceIdentifier, resourceID, role string) (Workspace, error) {
	item, err := s.workspaceService.Get(ctx, workspaceIdentifier)
	if err != nil {
		return Workspace{}, err
	}
	if strings.TrimSpace(resourceID) == "" {
		return Workspace{}, errors.New("resource ID is required")
	}
	if _, err := s.resourceRepository.Get(ctx, resourceID); err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			return Workspace{}, membershipContextError{message: fmt.Sprintf("resource %q not found", resourceID), cause: resource.ErrNotFound}
		}
		return Workspace{}, fmt.Errorf("get resource: %w", err)
	}

	membership := ResourceMembership{WorkspaceID: item.ID, ResourceID: resourceID, Role: role}
	if err := s.membershipRepository.Add(ctx, membership); err != nil {
		if errors.Is(err, ErrMembershipExists) {
			return Workspace{}, membershipContextError{
				message: fmt.Sprintf("resource %q is already in workspace %q", resourceID, item.Name),
				cause:   ErrMembershipExists,
			}
		}
		return Workspace{}, fmt.Errorf("add workspace resource: %w", err)
	}
	return item, nil
}

func (s *MembershipService) List(ctx context.Context, workspaceIdentifier string) (Workspace, []ResourceMembership, error) {
	item, err := s.workspaceService.Get(ctx, workspaceIdentifier)
	if err != nil {
		return Workspace{}, nil, err
	}
	memberships, err := s.membershipRepository.ListByWorkspace(ctx, item.ID)
	if err != nil {
		return Workspace{}, nil, fmt.Errorf("list workspace resources: %w", err)
	}
	sort.Slice(memberships, func(i, j int) bool {
		if memberships[i].Resource.Name == memberships[j].Resource.Name {
			return memberships[i].Resource.ID < memberships[j].Resource.ID
		}
		return memberships[i].Resource.Name < memberships[j].Resource.Name
	})
	return item, memberships, nil
}

func (s *MembershipService) Remove(ctx context.Context, workspaceIdentifier, resourceID string) (Workspace, error) {
	item, err := s.workspaceService.Get(ctx, workspaceIdentifier)
	if err != nil {
		return Workspace{}, err
	}
	if strings.TrimSpace(resourceID) == "" {
		return Workspace{}, errors.New("resource ID is required")
	}
	if err := s.membershipRepository.Remove(ctx, item.ID, resourceID); err != nil {
		if errors.Is(err, ErrMembershipNotFound) {
			return Workspace{}, membershipContextError{
				message: fmt.Sprintf("resource %q is not in workspace %q", resourceID, item.Name),
				cause:   ErrMembershipNotFound,
			}
		}
		return Workspace{}, fmt.Errorf("remove workspace resource: %w", err)
	}
	return item, nil
}

type membershipContextError struct {
	message string
	cause   error
}

func (e membershipContextError) Error() string { return e.message }
func (e membershipContextError) Unwrap() error { return e.cause }
