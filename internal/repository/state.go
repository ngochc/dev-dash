package repository

import (
	"context"
	"fmt"
	"sort"
)

func DeriveState(ctx context.Context, item Repository, inspector CheckoutInspector) (State, error) {
	if item.CheckoutPath == "" {
		return StateNotCloned, nil
	}
	inspection, err := inspector.Inspect(ctx, item.CheckoutPath, item.URL)
	if err != nil {
		return "", fmt.Errorf("inspect repository checkout %q: %w", item.CheckoutPath, err)
	}
	if !inspection.Exists {
		return StateMissing, nil
	}
	if inspection.Valid {
		return StateCloned, nil
	}
	return StateInvalid, nil
}

func List(ctx context.Context, items []Repository, inspector CheckoutInspector) ([]Listed, error) {
	listed := make([]Listed, len(items))
	for i, item := range items {
		state, err := DeriveState(ctx, item, inspector)
		if err != nil {
			return nil, err
		}
		listed[i] = Listed{Repository: item, State: state}
	}
	sort.Slice(listed, func(i, j int) bool {
		return listed[i].Repository.ExternalKey < listed[j].Repository.ExternalKey
	})
	return listed, nil
}

func Select(items []Repository, selectors []string, all bool) ([]Repository, error) {
	if all {
		selected := append([]Repository(nil), items...)
		sort.Slice(selected, func(i, j int) bool { return selected[i].ExternalKey < selected[j].ExternalKey })
		return selected, nil
	}

	byExternalKey := make(map[string]Repository, len(items))
	byName := make(map[string][]Repository, len(items))
	for _, item := range items {
		byExternalKey[item.ExternalKey] = item
		byName[item.Name] = append(byName[item.Name], item)
	}
	selected := make([]Repository, 0, len(selectors))
	seen := make(map[string]struct{}, len(selectors))
	for _, selector := range selectors {
		item, found := byExternalKey[selector]
		if !found {
			matches := byName[selector]
			switch len(matches) {
			case 0:
				return nil, fmt.Errorf("repository %q not found", selector)
			case 1:
				item = matches[0]
			default:
				return nil, fmt.Errorf("repository %q is ambiguous; use owner/repository", selector)
			}
		}
		if _, duplicate := seen[item.ResourceID]; duplicate {
			continue
		}
		seen[item.ResourceID] = struct{}{}
		selected = append(selected, item)
	}
	return selected, nil
}
