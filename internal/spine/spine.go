package spine

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	EnableEnv = "WORK_SPINE_DUAL_WRITE"
	DirEnv    = "POLIS_SPINE_DIR"
)

type RawEventEnvelope struct {
	ID          string         `json:"id"`
	TS          string         `json:"ts"`
	Kind        string         `json:"kind"`
	TraceID     string         `json:"trace_id"`
	SessionID   string         `json:"session_id"`
	RunID       string         `json:"run_id"`
	BeadID      *string        `json:"bead_id"`
	AgentID     *string        `json:"agent_id"`
	ParentRunID *string        `json:"parent_run_id"`
	Model       *string        `json:"model"`
	Data        map[string]any `json:"data"`
}

type Writer struct {
	baseDir string
	mu      sync.Mutex
}

func Enabled() bool {
	v := strings.TrimSpace(os.Getenv(EnableEnv))
	return v == "1" || strings.EqualFold(v, "true")
}

func DefaultDir() string {
	if v := strings.TrimSpace(os.Getenv(DirEnv)); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".polis", "spine", "events")
	}
	return filepath.Join(home, ".polis", "spine", "events")
}

func NewWriter(baseDir string) *Writer {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = DefaultDir()
	}
	return &Writer{baseDir: baseDir}
}

func (w *Writer) BaseDir() string {
	return w.baseDir
}

func (w *Writer) Write(env RawEventEnvelope) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(w.baseDir, 0o755); err != nil {
		return fmt.Errorf("create spine dir: %w", err)
	}

	date := extractDate(env.TS)
	target := filepath.Join(w.baseDir, date+".jsonl")
	f, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open spine file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal spine event: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append spine event: %w", err)
	}
	return nil
}

func ReadAll(baseDir string) ([]RawEventEnvelope, error) {
	var events []RawEventEnvelope
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return events, nil
	}

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return ignoreWalkEntryError(path, err)
		}
		if info.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return ignoreWalkEntryError(path, err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var env RawEventEnvelope
			if err := json.Unmarshal(scanner.Bytes(), &env); err != nil {
				continue
			}
			events = append(events, env)
		}
		if err := scanner.Err(); err != nil {
			return ignoreWalkEntryError(path, err)
		}
		return nil
	})
	return events, err
}

func ignoreWalkEntryError(_ string, _ error) error {
	return nil
}

func MintULID() string {
	var entropy [10]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		panic(fmt.Errorf("mint ulid entropy: %w", err))
	}

	ms := uint64(time.Now().UTC().UnixMilli())
	var raw [16]byte
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	copy(raw[6:], entropy[:])

	return encodeCrockford(raw)
}

func MintRunID() string {
	return "run-" + MintULID()
}

func MintSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("mint session id entropy: %w", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		uint64(b[10])<<40|uint64(b[11])<<32|uint64(b[12])<<24|uint64(b[13])<<16|uint64(b[14])<<8|uint64(b[15]),
	)
}

func extractDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return time.Now().UTC().Format("2006-01-02")
}

func encodeCrockford(raw [16]byte) string {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	value := new(big.Int).SetBytes(raw[:])
	divisor := big.NewInt(32)
	remainder := new(big.Int)

	var out [26]byte
	for i := 25; i >= 0; i-- {
		value.DivMod(value, divisor, remainder)
		out[i] = alphabet[remainder.Int64()]
	}
	return string(out[:])
}
