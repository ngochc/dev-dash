package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

var (
	ErrCLIUnavailable  = errors.New("GitHub CLI unavailable")
	ErrAuthentication  = errors.New("GitHub CLI authentication failed")
	ErrExternalCommand = errors.New("GitHub CLI command failed")
)

type Repository struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	NameWithOwner    string            `json:"nameWithOwner"`
	URL              string            `json:"url"`
	SSHURL           string            `json:"sshUrl"`
	IsArchived       bool              `json:"isArchived"`
	IsFork           bool              `json:"isFork"`
	DefaultBranchRef *DefaultBranchRef `json:"defaultBranchRef"`
}

type DefaultBranchRef struct {
	Name string `json:"name"`
}

type Owner struct {
	Login    string
	Personal bool
}

type CommandResult struct {
	Stdout []byte
	Stderr []byte
}

type CommandRunner interface {
	Run(context.Context, map[string]string, ...string) (CommandResult, error)
}

// ExecRunner executes the installed gh binary without modifying global configuration.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, environment map[string]string, args ...string) (CommandResult, error) {
	command := exec.CommandContext(ctx, "gh", args...)
	command.Env = commandEnvironment(os.Environ(), environment)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

type Client struct {
	runner CommandRunner
}

func NewClient(runner CommandRunner) *Client {
	return &Client{runner: runner}
}

func NewCLIClient() *Client {
	return NewClient(ExecRunner{})
}

func (c *Client) Validate(ctx context.Context, config Config) error {
	result, err := c.runner.Run(ctx, hostEnvironment(config.Host), "auth", "status", "--hostname", config.Host)
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, exec.ErrNotFound) {
		return clientError{
			message: "GitHub CLI is not installed.\n\nInstall it from:\n  https://cli.github.com/",
			cause:   errors.Join(ErrCLIUnavailable, err),
		}
	}
	return clientError{
		message: fmt.Sprintf("GitHub CLI is not authenticated for %s.\n\nAuthenticate with:\n  gh auth login --hostname %s%s", config.Host, config.Host, commandDetail(result.Stderr)),
		cause:   errors.Join(ErrAuthentication, err),
	}
}

func (c *Client) Discover(ctx context.Context, config Config) ([]Repository, error) {
	fields := "id,name,nameWithOwner,url,sshUrl,isArchived,isFork,defaultBranchRef"
	result, err := c.runner.Run(
		ctx,
		hostEnvironment(config.Host),
		"repo", "list", config.Organization,
		"--limit", "1000",
		"--no-archived",
		"--json", fields,
	)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, clientError{
			message: "GitHub repository discovery failed" + commandDetail(result.Stderr),
			cause:   errors.Join(ErrExternalCommand, err),
		}
	}
	var repositories []Repository
	if err := json.Unmarshal(result.Stdout, &repositories); err != nil {
		return nil, fmt.Errorf("parse GitHub repository list: %w", err)
	}
	for _, repository := range repositories {
		if repository.ID == "" || repository.Name == "" || repository.NameWithOwner == "" || repository.URL == "" {
			return nil, fmt.Errorf("parse GitHub repository list: repository identity is incomplete")
		}
	}
	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].NameWithOwner < repositories[j].NameWithOwner
	})
	return repositories, nil
}

func (c *Client) DiscoverOwners(ctx context.Context, config Config) ([]Owner, error) {
	userResult, err := c.runner.Run(ctx, hostEnvironment(config.Host), "api", "user")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, clientError{
			message: "GitHub owner discovery failed" + commandDetail(userResult.Stderr),
			cause:   errors.Join(ErrExternalCommand, err),
		}
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(userResult.Stdout, &user); err != nil {
		return nil, fmt.Errorf("parse GitHub authenticated user: %w", err)
	}
	user.Login = strings.TrimSpace(user.Login)
	if user.Login == "" {
		return nil, fmt.Errorf("parse GitHub authenticated user: identity is empty")
	}

	organizationResult, err := c.runner.Run(ctx, hostEnvironment(config.Host), "api", "--paginate", "user/orgs")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, clientError{
			message: "GitHub organization discovery failed" + commandDetail(organizationResult.Stderr),
			cause:   errors.Join(ErrExternalCommand, err),
		}
	}

	ownersByLogin := map[string]Owner{
		strings.ToLower(user.Login): {Login: user.Login, Personal: true},
	}
	decoder := json.NewDecoder(bytes.NewReader(organizationResult.Stdout))
	for {
		var page []struct {
			Login string `json:"login"`
		}
		if err := decoder.Decode(&page); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parse GitHub organizations: %w", err)
		}
		for _, organization := range page {
			login := strings.TrimSpace(organization.Login)
			if login == "" {
				return nil, fmt.Errorf("parse GitHub organizations: identity is empty")
			}
			key := strings.ToLower(login)
			if _, exists := ownersByLogin[key]; !exists {
				ownersByLogin[key] = Owner{Login: login}
			}
		}
	}

	owners := make([]Owner, 0, len(ownersByLogin))
	for _, owner := range ownersByLogin {
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(i, j int) bool {
		left, right := strings.ToLower(owners[i].Login), strings.ToLower(owners[j].Login)
		if left == right {
			return owners[i].Login < owners[j].Login
		}
		return left < right
	})
	return owners, nil
}

func (c *Client) Clone(ctx context.Context, config Config, repository, destination string) error {
	result, err := c.runner.Run(ctx, hostEnvironment(config.Host), "repo", "clone", repository, destination)
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return clientError{
		message: fmt.Sprintf("clone %s failed%s", repository, commandDetail(result.Stderr)),
		cause:   errors.Join(ErrExternalCommand, err),
	}
}

func hostEnvironment(host string) map[string]string {
	if strings.EqualFold(host, "github.com") {
		return nil
	}
	return map[string]string{"GH_HOST": host}
}

func commandEnvironment(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	environment := make([]string, 0, len(base)+len(overrides))
	for _, variable := range base {
		name, _, found := strings.Cut(variable, "=")
		if _, overridden := overrides[name]; found && overridden {
			continue
		}
		environment = append(environment, variable)
	}
	for name, value := range overrides {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func commandDetail(stderr []byte) string {
	detail := strings.TrimSpace(string(stderr))
	if detail == "" {
		return ""
	}
	return ": " + detail
}

type clientError struct {
	message string
	cause   error
}

func (e clientError) Error() string { return e.message }
func (e clientError) Unwrap() error { return e.cause }
