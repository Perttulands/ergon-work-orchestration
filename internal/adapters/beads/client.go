package beads

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Perttulands/polis-utils/brclient"
)

var sharedClient = brclient.New()

func Available() bool {
	return sharedClient.Available()
}

func Create(title, repo string) (string, error) {
	result, err := sharedClient.Run(context.Background(), brclient.Invocation{
		Args: []string{"create", title, "--silent"},
		Dir:  repo,
	})
	if errors.Is(err, brclient.ErrUnavailable) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("br create: %s%s: %w", strings.TrimSpace(string(result.Stdout)), trimmedStderr(result.Stderr), err)
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}

func Close(id, reason, repo string) error {
	result, err := sharedClient.Run(context.Background(), brclient.Invocation{
		Args: []string{"close", id, "--reason", reason},
		Dir:  repo,
	})
	if errors.Is(err, brclient.ErrUnavailable) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("br close %s: %s%s: %w", id, strings.TrimSpace(string(result.Stdout)), trimmedStderr(result.Stderr), err)
	}
	return nil
}

func ShowJSON(id, repo string) ([]byte, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	result, err := sharedClient.Run(context.Background(), brclient.Invocation{
		Args: []string{"show", id, "--json"},
		Dir:  repo,
	})
	if errors.Is(err, brclient.ErrUnavailable) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("br show %s: %s%s: %w", id, strings.TrimSpace(string(result.Stdout)), trimmedStderr(result.Stderr), err)
	}
	return result.Stdout, nil
}

func ShowText(id, repo string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", nil
	}
	result, err := sharedClient.Run(context.Background(), brclient.Invocation{
		Args: []string{"show", id},
		Dir:  repo,
	})
	if errors.Is(err, brclient.ErrUnavailable) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("br show %s: %s%s: %w", id, strings.TrimSpace(string(result.Stdout)), trimmedStderr(result.Stderr), err)
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}

func SearchClosed(query, repo string, limit int) ([]byte, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("no search query available")
	}
	result, err := sharedClient.Run(context.Background(), brclient.Invocation{
		Args: []string{"search", query, "--json", "--limit", fmt.Sprintf("%d", limit), "--status", "closed"},
		Dir:  repo,
	})
	if errors.Is(err, brclient.ErrUnavailable) {
		return nil, fmt.Errorf("br not on PATH")
	}
	if err != nil {
		return nil, fmt.Errorf("br search: %s%s: %w", strings.TrimSpace(string(result.Stdout)), trimmedStderr(result.Stderr), err)
	}
	return result.Stdout, nil
}

func ListClosed(repo string) ([]byte, error) {
	result, err := sharedClient.Run(context.Background(), brclient.Invocation{
		Args: []string{"list", "--status", "closed", "--json"},
		Dir:  repo,
	})
	if errors.Is(err, brclient.ErrUnavailable) {
		return nil, fmt.Errorf("br not on PATH")
	}
	if err != nil {
		return nil, fmt.Errorf("br list: %s%s: %w", strings.TrimSpace(string(result.Stdout)), trimmedStderr(result.Stderr), err)
	}
	return result.Stdout, nil
}

func trimmedStderr(stderr []byte) string {
	text := strings.TrimSpace(string(stderr))
	if text == "" {
		return ""
	}
	return ": " + text
}
