package worker

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// RuntimeConfigEnv points to a JSON file with runtime and agent profiles.
	RuntimeConfigEnv = "WORK_RUNTIME_CONFIG"
)

//go:embed worker_profiles.default.json
var embeddedProfilesJSON []byte

type profileConfig struct {
	DefaultRuntime string                  `json:"default_runtime"`
	Runtimes       map[string]runtimeSpec  `json:"runtimes"`
	Agents         map[string]agentProfile `json:"agents"`
}

type runtimeSpec struct {
	Command       string   `json:"command"`
	Args          []string `json:"args"`
	Model         string   `json:"model"`
	ReadyPatterns []string `json:"ready_patterns"`
	TrustPatterns []string `json:"trust_patterns"`
}

type agentProfile struct {
	Runtime string `json:"runtime"`
}

func loadProfiles() (*profileConfig, error) {
	raw, err := readProfilesJSON()
	if err != nil {
		return nil, fmt.Errorf("read runtime profiles: %w", err)
	}

	var cfg profileConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse runtime profiles: %w", err)
	}

	cfg.DefaultRuntime = strings.ToLower(strings.TrimSpace(cfg.DefaultRuntime))
	if cfg.DefaultRuntime == "" {
		return nil, fmt.Errorf("runtime profiles: default_runtime is required")
	}
	if len(cfg.Runtimes) == 0 {
		return nil, fmt.Errorf("runtime profiles: runtimes is empty")
	}

	normalized := make(map[string]runtimeSpec, len(cfg.Runtimes))
	for name, spec := range cfg.Runtimes {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			return nil, fmt.Errorf("runtime profiles: runtime name cannot be empty")
		}
		if strings.TrimSpace(spec.Command) == "" {
			return nil, fmt.Errorf("runtime profiles: runtime %q has empty command", key)
		}
		normalized[key] = spec
	}
	cfg.Runtimes = normalized

	if _, ok := cfg.Runtimes[cfg.DefaultRuntime]; !ok {
		return nil, fmt.Errorf("runtime profiles: default_runtime %q not found in runtimes", cfg.DefaultRuntime)
	}

	for agent, prof := range cfg.Agents {
		rt := strings.ToLower(strings.TrimSpace(prof.Runtime))
		if rt == "" {
			return nil, fmt.Errorf("runtime profiles: agent %q has empty runtime", agent)
		}
		if _, ok := cfg.Runtimes[rt]; !ok {
			return nil, fmt.Errorf("runtime profiles: agent %q references unknown runtime %q", agent, rt)
		}
	}

	return &cfg, nil
}

func readProfilesJSON() ([]byte, error) {
	if p := strings.TrimSpace(os.Getenv(RuntimeConfigEnv)); p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read %s (%s): %w", RuntimeConfigEnv, p, err)
		}
		return data, nil
	}

	home, err := os.UserHomeDir()
	if err == nil {
		userConfig := filepath.Join(home, ".work", "worker_profiles.json")
		if data, readErr := os.ReadFile(userConfig); readErr == nil {
			return data, nil
		}
	}

	return embeddedProfilesJSON, nil
}

func resolveRuntimeProfile(requestedRuntime, agentName string) (string, runtimeSpec, error) {
	cfg, err := loadProfiles()
	if err != nil {
		return "", runtimeSpec{}, fmt.Errorf("load runtime profiles: %w", err)
	}

	runtime := strings.ToLower(strings.TrimSpace(requestedRuntime))
	if runtime == "" {
		agentKey := strings.ToLower(strings.TrimSpace(agentName))
		if agent, ok := cfg.Agents[agentKey]; ok {
			runtime = strings.ToLower(strings.TrimSpace(agent.Runtime))
		}
	}
	if runtime == "" {
		runtime = cfg.DefaultRuntime
	}

	spec, ok := cfg.Runtimes[runtime]
	if !ok {
		return "", runtimeSpec{}, fmt.Errorf("unsupported runtime %q (check %s or ~/.work/worker_profiles.json)", runtime, RuntimeConfigEnv)
	}
	return runtime, spec, nil
}

var safeShellWord = regexp.MustCompile(`^[A-Za-z0-9_./:=+-]+$`)

func shellJoin(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		if safeShellWord.MatchString(p) {
			quoted = append(quoted, p)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(p, "'", `'"'"'`)+"'")
	}
	return strings.Join(quoted, " ")
}

func buildLaunchCommand(spec runtimeSpec) (string, error) {
	bin := strings.TrimSpace(spec.Command)
	if bin == "" {
		return "", fmt.Errorf("runtime command is empty")
	}
	parts := make([]string, 0, 1+len(spec.Args))
	parts = append(parts, "command", bin)
	parts = append(parts, spec.Args...)
	return shellJoin(parts), nil
}

// ResolveRuntime resolves runtime by explicit flag, then agent mapping, then default profile.
func ResolveRuntime(requestedRuntime, agentName string) (string, error) {
	name, _, err := resolveRuntimeProfile(requestedRuntime, agentName)
	return name, err
}

// ModelForRuntime returns the configured model label for a runtime selection.
func ModelForRuntime(requestedRuntime, agentName string) string {
	_, spec, err := resolveRuntimeProfile(requestedRuntime, agentName)
	if err != nil {
		return strings.TrimSpace(requestedRuntime)
	}
	if strings.TrimSpace(spec.Model) == "" {
		return strings.TrimSpace(requestedRuntime)
	}
	return spec.Model
}
