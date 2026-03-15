package relay

import (
	"fmt"
	"os/exec"
	"strings"
)

func Available() bool {
	_, err := exec.LookPath("relay")
	return err == nil
}

func Heartbeat(agent string) error {
	if !Available() {
		return nil
	}
	cmd := exec.Command("relay", "heartbeat", "--agent", agent)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("relay heartbeat %s: %s: %w", agent, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func Register(agent string) error {
	if !Available() {
		return nil
	}
	if strings.TrimSpace(agent) == "" {
		return nil
	}
	cmd := exec.Command("relay", "register", agent)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("relay register %s: %s: %w", agent, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func Send(from, to, message, thread, msgType, payload string) error {
	if !Available() {
		return nil
	}
	args := []string{"send", to, message, "--agent", from}
	if thread != "" {
		args = append(args, "--thread", thread)
	}
	if msgType != "" {
		args = append(args, "--type", msgType)
	}
	if payload != "" {
		args = append(args, "--payload", payload)
	}
	cmd := exec.Command("relay", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("relay send %s->%s: %s: %w", from, to, strings.TrimSpace(string(out)), err)
	}
	return nil
}
