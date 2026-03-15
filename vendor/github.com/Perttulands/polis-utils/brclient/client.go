package brclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrUnavailable = errors.New("br binary not found on PATH")

type Client struct {
	Bin string
}

type Invocation struct {
	Args []string
	Dir  string
	Env  []string
}

type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type CommandError struct {
	Bin    string
	Args   []string
	Result Result
	Err    error
}

func New() *Client {
	return &Client{Bin: "br"}
}

func (c *Client) Available() bool {
	_, err := exec.LookPath(c.binary())
	return err == nil
}

func (c *Client) Run(ctx context.Context, inv Invocation) (Result, error) {
	if len(inv.Args) == 0 {
		return Result{}, fmt.Errorf("br invocation requires args")
	}

	path, err := resolveBinary(c.binary(), inv.Env)
	if err != nil {
		return Result{}, ErrUnavailable
	}

	cmd := exec.CommandContext(ctx, path, inv.Args...)
	if inv.Dir != "" {
		cmd.Dir = inv.Dir
	}
	if len(inv.Env) > 0 {
		cmd.Env = append(os.Environ(), inv.Env...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	result := Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode(err),
	}
	if err != nil {
		return result, &CommandError{
			Bin:    c.binary(),
			Args:   append([]string(nil), inv.Args...),
			Result: result,
			Err:    err,
		}
	}
	return result, nil
}

func resolveBinary(bin string, env []string) (string, error) {
	if path, ok := envValue("PATH", env); ok {
		return lookPathWithPATH(bin, path)
	}
	return exec.LookPath(bin)
}

func envValue(key string, env []string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix), true
		}
	}
	return "", false
}

func lookPathWithPATH(bin, pathValue string) (string, error) {
	if strings.ContainsRune(bin, filepath.Separator) {
		if isExecutable(bin) {
			return bin, nil
		}
		return "", exec.ErrNotFound
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, bin)
		if isExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func (c *Client) binary() string {
	if c == nil || c.Bin == "" {
		return "br"
	}
	return c.Bin
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s %v: %v", e.Bin, e.Args, e.Err)
}

func (e *CommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
