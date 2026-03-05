package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type failureMode int

const (
	failOpen failureMode = iota
	failClosed
)

type failurePolicy struct {
	name string
	mode failureMode
}

const (
	stepBrCreate          = "br_create"
	stepBrAgentWorking    = "br_agent_state_working"
	stepRelayRegister     = "relay_register"
	stepRelayHeartbeat    = "relay_heartbeat"
	stepContextGather     = "context_gather"
	stepTraceOpen         = "trace_open"
	stepGateCheck         = "gate_check"
	stepIndexOpen         = "index_open"
	stepIndexRecord       = "index_record"
	stepFeedbackCollect   = "feedback_collect"
	stepLoopIngest        = "loop_ingest"
	stepExperienceAppend  = "experience_append"
	stepCloseReasonDerive = "close_reason_derive"
	stepBrClose           = "br_close"
	stepRelaySendAthena   = "relay_send_athena"
	stepRelaySendNotify   = "relay_send_notify"
	stepBrAgentIdle       = "br_agent_state_idle"
)

var policies = map[string]failurePolicy{
	stepBrCreate:          {name: "br create", mode: failOpen},
	stepBrAgentWorking:    {name: "br agent state (working)", mode: failOpen},
	stepRelayRegister:     {name: "relay register", mode: failOpen},
	stepRelayHeartbeat:    {name: "relay heartbeat", mode: failOpen},
	stepContextGather:     {name: "context gather", mode: failOpen},
	stepTraceOpen:         {name: "trace open", mode: failOpen},
	stepGateCheck:         {name: "gate check", mode: failOpen},
	stepIndexOpen:         {name: "index open", mode: failOpen},
	stepIndexRecord:       {name: "index record", mode: failOpen},
	stepFeedbackCollect:   {name: "feedback collect", mode: failOpen},
	stepLoopIngest:        {name: "loop ingest", mode: failOpen},
	stepExperienceAppend:  {name: "experience append", mode: failOpen},
	stepCloseReasonDerive: {name: "derive close reason", mode: failOpen},
	stepBrClose:           {name: "br close", mode: failOpen},
	stepRelaySendAthena:   {name: "relay send athena", mode: failOpen},
	stepRelaySendNotify:   {name: "relay send notify", mode: failOpen},
	stepBrAgentIdle:       {name: "br agent state (idle)", mode: failOpen},
}

func strictMode(cmd *cobra.Command) bool {
	if cmd != nil {
		if strict, err := cmd.Flags().GetBool("strict"); err == nil && strict {
			return true
		}
	}
	raw, ok := os.LookupEnv("WORK_STRICT")
	if !ok {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(raw))
	return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
}

func applyFailurePolicy(cmd *cobra.Command, step string, err error) error {
	if err == nil {
		return nil
	}

	policy, ok := policies[step]
	if !ok {
		policy = failurePolicy{name: step, mode: failOpen}
	}

	if policy.mode == failClosed || strictMode(cmd) {
		return fmt.Errorf("%s: %w", policy.name, err)
	}

	if cmd != nil {
		cmd.Printf("  Warning: %s: %v\n", policy.name, err)
	}
	return nil
}
