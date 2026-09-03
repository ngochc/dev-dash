package git

import (
	"fmt"
	"net/url"
	"strings"
)

type RemoteIdentity struct {
	Host       string
	Owner      string
	Repository string
}

func NormalizeRemote(remote string) (RemoteIdentity, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return RemoteIdentity{}, fmt.Errorf("git remote is empty")
	}

	var host, remotePath string
	if !strings.Contains(remote, "://") {
		before, after, found := strings.Cut(remote, ":")
		if found && strings.Contains(before, "@") {
			_, host, _ = strings.Cut(before, "@")
			remotePath = after
		}
	}
	if host == "" {
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Hostname() == "" {
			return RemoteIdentity{}, fmt.Errorf("invalid Git remote %q", remote)
		}
		host = parsed.Hostname()
		remotePath = parsed.Path
	}

	remotePath = strings.Trim(strings.TrimSpace(remotePath), "/")
	remotePath = strings.TrimSuffix(remotePath, ".git")
	parts := strings.Split(remotePath, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RemoteIdentity{}, fmt.Errorf("invalid Git remote %q", remote)
	}
	return RemoteIdentity{
		Host:       strings.ToLower(host),
		Owner:      parts[0],
		Repository: parts[1],
	}, nil
}
