package gate

import (
	"encoding/json"
	"os/exec"
	"strings"
)

type Result struct {
	Pass  bool    `json:"pass"`
	Score float64 `json:"score"`
	Raw   string
}

func Available() bool {
	_, err := exec.LookPath("gate")
	return err == nil
}

func Check(repo, citizen string) (*Result, error) {
	if !Available() {
		return nil, nil
	}

	args := []string{"check", ".", "--json"}
	if citizen != "" {
		args = append(args, "--citizen", citizen)
	}
	cmd := exec.Command("gate", args...)
	cmd.Dir = repo
	out, err := cmd.Output()
	raw := strings.TrimSpace(string(out))

	var result Result
	if jsonErr := json.Unmarshal(out, &result); jsonErr != nil {
		result.Pass = err == nil
		result.Raw = raw
		return &result, nil
	}
	result.Raw = raw
	return &result, nil
}
