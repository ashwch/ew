package hook

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ashwch/ew/internal/appdirs"
	"github.com/ashwch/ew/internal/safety"
)

const eventsFileName = "events.jsonl"
const maxCommandLength = 8192

type Event struct {
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	CWD       string `json:"cwd"`
	Shell     string `json:"shell"`
	SessionID string `json:"session_id,omitempty"`
	Timestamp string `json:"timestamp"`
}

func RecordEvent(ev Event) error {
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	ev.Command = strings.TrimSpace(ev.Command)
	if ev.Command == "" {
		return fmt.Errorf("command cannot be empty")
	}
	if shouldIgnoreCommand(ev.Command) {
		return nil
	}
	ev.Command = strings.TrimSpace(safety.RedactText(ev.Command))
	if ev.Command == "" {
		return fmt.Errorf("command cannot be empty")
	}
	if len(ev.Command) > maxCommandLength {
		ev.Command = ev.Command[:maxCommandLength]
	}

	if _, err := appdirs.EnsureStateDir(); err != nil {
		return err
	}
	path, err := appdirs.StateFilePath(eventsFileName)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("could not open events file: %w", err)
	}
	defer f.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("could not secure events file permissions: %w", err)
	}

	encoded, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("could not serialize event: %w", err)
	}
	if _, err := f.WriteString(string(encoded) + "\n"); err != nil {
		return fmt.Errorf("could not write event: %w", err)
	}
	return nil
}

func LatestFailure(sessionID string) (*Event, error) {
	path, err := appdirs.StateFilePath(eventsFileName)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not read events file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var latest *Event
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.ExitCode == 0 {
			continue
		}
		if isSyntheticSessionID(ev.SessionID) {
			continue
		}
		if sessionID != "" && ev.SessionID != sessionID {
			continue
		}
		candidate := ev
		latest = &candidate
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("could not scan events file: %w", err)
	}
	return latest, nil
}

func isSyntheticSessionID(sessionID string) bool {
	normalized := strings.ToLower(strings.TrimSpace(sessionID))
	if normalized == "" {
		return false
	}
	return strings.HasPrefix(normalized, "ew-test") || strings.HasPrefix(normalized, "ew-prov-test")
}

func shouldIgnoreCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return true
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return true
	}

	first := primaryCommandToken(fields)
	first = strings.ToLower(filepath.Base(first))
	if first == "ew" || first == "_ew" {
		return true
	}

	low := strings.ToLower(trimmed)
	if strings.HasPrefix(low, "_ew hook-record") {
		return true
	}
	if strings.Contains(low, "go run ./cmd/ew") || strings.Contains(low, "go run ./cmd/_ew") {
		return true
	}
	return false
}

func primaryCommandToken(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	idx := 0
	for idx < len(fields) {
		token := strings.TrimSpace(fields[idx])
		if token == "" {
			idx++
			continue
		}
		if isEnvAssignmentToken(token) {
			idx++
			continue
		}
		base := strings.ToLower(filepath.Base(token))
		switch base {
		case "env":
			idx++
			for idx < len(fields) {
				next := strings.TrimSpace(fields[idx])
				if next == "" {
					idx++
					continue
				}
				if strings.HasPrefix(next, "-") || isEnvAssignmentToken(next) {
					idx++
					continue
				}
				break
			}
			continue
		case "sudo", "command", "time", "nohup", "builtin":
			idx++
			for idx < len(fields) {
				next := strings.TrimSpace(fields[idx])
				if next == "" {
					idx++
					continue
				}
				if strings.HasPrefix(next, "-") {
					idx++
					continue
				}
				break
			}
			continue
		default:
			return token
		}
	}
	return fields[0]
}

func isEnvAssignmentToken(token string) bool {
	if strings.HasPrefix(token, "-") {
		return false
	}
	eq := strings.IndexRune(token, '=')
	if eq <= 0 {
		return false
	}
	return strings.IndexAny(token[:eq], "/\\") == -1
}
