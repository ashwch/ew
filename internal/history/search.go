package history

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Entry struct {
	Command   string
	Timestamp time.Time
	Source    string
	order     int
	approxTS  bool
}

type Match struct {
	Command   string  `json:"command"`
	Score     float64 `json:"score"`
	Source    string  `json:"source"`
	Timestamp string  `json:"timestamp,omitempty"`
}

const maxHistoryLineBytes = 1024 * 1024

var promptClockSuffix = regexp.MustCompile(`\s{2,}\d{1,2}:\d{2}$`)

func LoadEntries() ([]Entry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine home directory: %w", err)
	}

	var entries []Entry
	nextOrder := 0
	paths := []struct {
		path   string
		loader func(string) ([]Entry, error)
	}{
		{filepath.Join(home, ".zsh_history"), loadZshHistory},
		{filepath.Join(home, ".bash_history"), loadBashHistory},
		{filepath.Join(home, ".local", "share", "fish", "fish_history"), loadFishHistory},
	}

	for _, p := range paths {
		if _, err := os.Stat(p.path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		loaded, err := p.loader(p.path)
		if err != nil {
			continue
		}
		for _, entry := range loaded {
			entry.order = nextOrder
			nextOrder++
			entries = append(entries, entry)
		}
	}

	if len(entries) == 0 {
		return nil, nil
	}

	entries = dedupeEntries(entries)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Timestamp.Equal(entries[j].Timestamp) {
			return entries[i].order > entries[j].order
		}
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})
	if len(entries) > 12000 {
		entries = entries[:12000]
	}
	return entries, nil
}

func LatestEntry(maxAge time.Duration) (*Entry, error) {
	entries, err := LoadEntries()
	if err != nil {
		return nil, err
	}
	for _, latest := range entries {
		if latest.Timestamp.IsZero() || latest.approxTS {
			continue
		}
		if maxAge > 0 {
			age := time.Since(latest.Timestamp)
			if age < 0 || age > maxAge {
				continue
			}
		}
		copy := latest
		return &copy, nil
	}
	return nil, nil
}

func Search(query string, limit int) ([]Match, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if limit <= 0 {
		limit = 8
	}

	entries, err := LoadEntries()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	tokens := splitTokens(queryLower)

	matches := make([]Match, 0, len(entries))
	now := time.Now()
	for idx, entry := range entries {
		cmdLower := strings.ToLower(entry.Command)
		score := scoreCommand(queryLower, tokens, cmdLower, idx, now.Sub(entry.Timestamp))
		if score <= 0 {
			continue
		}
		matches = append(matches, Match{
			Command:   entry.Command,
			Score:     score,
			Source:    entry.Source,
			Timestamp: entry.Timestamp.Format(time.RFC3339),
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Timestamp > matches[j].Timestamp
		}
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

func scoreCommand(query string, tokens []string, cmd string, recencyIndex int, age time.Duration) float64 {
	if cmd == "" {
		return 0
	}
	score := 0.0

	if strings.Contains(cmd, query) {
		score += 12
	}
	if strings.HasPrefix(cmd, query) {
		score += 8
	}

	matched := 0
	lastTokenPos := -1
	orderedMatches := 0
	for _, token := range tokens {
		if token == "" {
			continue
		}
		pos := tokenIndex(cmd, token)
		if pos >= 0 {
			matched++
			score += 4
			if lastTokenPos >= 0 && pos > lastTokenPos {
				orderedMatches++
			}
			lastTokenPos = pos
		}
	}
	minMatchedTokens := minimumTokenMatches(tokens)
	if matched < minMatchedTokens {
		return 0
	}
	if orderedMatches > 0 {
		score += float64(orderedMatches) * 1.2
	}
	score -= unmatchedDistinctiveTokenPenalty(tokens, cmd)

	if len(cmd) > 160 {
		score -= 2
	}
	if len(cmd) > 280 {
		score -= 3
	}
	if strings.Count(cmd, "/") >= 4 {
		score -= 1.5
	}

	if age < 24*time.Hour {
		score += 4
	} else if age < 7*24*time.Hour {
		score += 2
	}

	if recencyIndex < 20 {
		score += 2
	} else if recencyIndex < 200 {
		score += 1
	}

	if score <= 0 {
		return 0
	}
	return score
}

func unmatchedDistinctiveTokenPenalty(tokens []string, cmd string) float64 {
	penalty := 0.0
	for _, token := range tokens {
		if len(token) < 8 {
			continue
		}
		if tokenIndex(cmd, token) >= 0 {
			continue
		}
		penalty += 2.8
	}
	return penalty
}

func minimumTokenMatches(tokens []string) int {
	count := 0
	for _, token := range tokens {
		if strings.TrimSpace(token) != "" {
			count++
		}
	}
	if count == 0 {
		return 0
	}
	switch {
	case count >= 6:
		return 3
	case count >= 2:
		return 2
	default:
		return 1
	}
}

func splitTokens(query string) []string {
	stopwords := map[string]struct{}{
		"the": {}, "for": {}, "and": {}, "with": {}, "from": {}, "into": {}, "onto": {}, "that": {}, "this": {},
		"you": {}, "your": {}, "can": {}, "could": {}, "how": {}, "what": {}, "when": {}, "where": {}, "why": {},
		"are": {}, "is": {}, "to": {}, "me": {}, "my": {}, "find": {}, "search": {}, "please": {}, "help": {},
		"command": {}, "commands": {}, "run": {}, "execute": {}, "path": {}, "paths": {}, "file": {}, "files": {}, "location": {},
	}
	parts := strings.FieldsFunc(query, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '-' || r == '_' || r == ':' || r == '/'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		token := strings.ToLower(strings.Trim(strings.TrimSpace(p), `"'.,!?;:()[]{}<>`))
		if len(token) < 3 {
			continue
		}
		if _, blocked := stopwords[token]; blocked {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func dedupeEntries(entries []Entry) []Entry {
	latestByCommand := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		cmd := normalizeHistoryCommand(entry.Command)
		if cmd == "" {
			continue
		}
		if isSensitiveCommand(cmd) {
			continue
		}
		if isLikelyShellOutput(cmd) {
			continue
		}
		if isInternalCommand(cmd) {
			continue
		}
		key := strings.ToLower(cmd)
		entry.Command = cmd

		current, ok := latestByCommand[key]
		if !ok || entry.Timestamp.After(current.Timestamp) || (entry.Timestamp.Equal(current.Timestamp) && entry.order > current.order) {
			latestByCommand[key] = entry
		}
	}
	out := make([]Entry, 0, len(latestByCommand))
	for _, entry := range latestByCommand {
		out = append(out, entry)
	}
	return out
}

func normalizeHistoryCommand(command string) string {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return ""
	}
	for strings.HasSuffix(cmd, `\`) {
		cmd = strings.TrimSpace(strings.TrimSuffix(cmd, `\`))
	}
	cmd = strings.TrimSpace(promptClockSuffix.ReplaceAllString(cmd, ""))
	return cmd
}

func loadZshHistory(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	untimedIndexes := make([]int, 0, 32)
	scanner := newHistoryScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		timestamp := time.Time{}
		approx := true
		command := line
		if strings.HasPrefix(line, ": ") {
			parts := strings.SplitN(line, ";", 2)
			if len(parts) == 2 {
				meta := strings.TrimPrefix(parts[0], ": ")
				metaParts := strings.Split(meta, ":")
				if len(metaParts) > 0 {
					if unixTs, err := parseUnix(metaParts[0]); err == nil {
						timestamp = time.Unix(unixTs, 0).UTC()
						approx = false
					}
				}
				command = parts[1]
			}
		}
		entries = append(entries, Entry{
			Command:   command,
			Timestamp: timestamp,
			Source:    "zsh",
			approxTS:  approx,
		})
		if timestamp.IsZero() {
			untimedIndexes = append(untimedIndexes, len(entries)-1)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(untimedIndexes) > 0 {
		start := time.Now().UTC().Add(-time.Duration(len(untimedIndexes)) * time.Second)
		for i, idx := range untimedIndexes {
			entries[idx].Timestamp = start.Add(time.Duration(i) * time.Second)
			entries[idx].approxTS = true
		}
	}
	return entries, nil
}

func loadBashHistory(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := newHistoryScanner(f)
	untimedIndexes := make([]int, 0, 32)
	var pendingTimestamp time.Time
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if unixTs, err := parseUnix(strings.TrimPrefix(line, "#")); err == nil {
				pendingTimestamp = time.Unix(unixTs, 0).UTC()
			} else {
				pendingTimestamp = time.Time{}
			}
			continue
		}
		timestamp := pendingTimestamp
		pendingTimestamp = time.Time{}
		approx := false
		entries = append(entries, Entry{
			Command:   line,
			Timestamp: timestamp,
			Source:    "bash",
			approxTS:  approx,
		})
		if timestamp.IsZero() {
			entries[len(entries)-1].approxTS = true
			untimedIndexes = append(untimedIndexes, len(entries)-1)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(untimedIndexes) > 0 {
		start := time.Now().UTC().Add(-time.Duration(len(untimedIndexes)) * time.Second)
		for i, idx := range untimedIndexes {
			entries[idx].Timestamp = start.Add(time.Duration(i) * time.Second)
		}
	}
	return entries, nil
}

func loadFishHistory(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := newHistoryScanner(f)
	currentCommand := ""
	currentTimestamp := time.Time{}
	flush := func() {
		if strings.TrimSpace(currentCommand) == "" {
			return
		}
		timestamp := currentTimestamp
		approx := false
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
			approx = true
		}
		entries = append(entries, Entry{
			Command:   strings.TrimSpace(currentCommand),
			Timestamp: timestamp,
			Source:    "fish",
			approxTS:  approx,
		})
		currentCommand = ""
		currentTimestamp = time.Time{}
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "- cmd:") {
			flush()
			currentCommand = strings.TrimSpace(strings.TrimPrefix(line, "- cmd:"))
			continue
		}
		if strings.HasPrefix(line, "when:") {
			if unixTs, err := parseUnix(strings.TrimSpace(strings.TrimPrefix(line, "when:"))); err == nil {
				currentTimestamp = time.Unix(unixTs, 0).UTC()
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return entries, nil
}

func parseUnix(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &v)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func isSensitiveCommand(command string) bool {
	low := strings.ToLower(strings.TrimSpace(command))
	patterns := []string{
		"export aws_session_token=",
		"export aws_secret_access_key=",
		"export aws_access_key_id=",
		"aws_session_token=",
		"aws_secret_access_key=",
		"aws_access_key_id=",
		"password=",
		"passwd",
		"token=",
		"secret=",
		"private_key",
		"authorization: bearer",
	}
	for _, pattern := range patterns {
		if strings.Contains(low, pattern) {
			return true
		}
	}
	return false
}

func isLikelyShellOutput(command string) bool {
	trimmed := strings.TrimSpace(command)
	low := strings.ToLower(trimmed)
	if low == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(trimmed)
	if r == utf8.RuneError || r > unicode.MaxASCII {
		return true
	}
	if !isLikelyCommandStarter(r) {
		return true
	}
	if isEnumeratedOutputLine(low) {
		return true
	}
	prefixes := []string{
		"zsh:",
		"bash:",
		"fish:",
		"usage:",
		"error:",
		"fatal:",
		"suggested command:",
		"reason:",
		"source:",
		"tip:",
		"top matches for:",
		"cancelled. command not executed.",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(low, prefix) {
			return true
		}
	}
	if strings.Contains(low, "command not found") {
		return true
	}
	if strings.Contains(low, "[error]") {
		return true
	}
	if strings.Contains(low, "do you want to") {
		return true
	}
	if strings.Contains(low, "worktree created") || strings.Contains(low, "created successfully") {
		return true
	}
	if strings.Contains(low, "run this command? [y/n]") {
		return true
	}
	fields := strings.Fields(low)
	if len(fields) >= 2 {
		if (fields[1] == "error" || fields[1] == "warn" || fields[1] == "warning") && isLikelyToolName(fields[0]) {
			return true
		}
	}
	if looksLikeNarrativeOutput(trimmed, low) {
		return true
	}
	return false
}

func isEnumeratedOutputLine(low string) bool {
	trimmed := strings.TrimSpace(low)
	if trimmed == "" {
		return false
	}
	idx := 0
	for idx < len(trimmed) && trimmed[idx] >= '0' && trimmed[idx] <= '9' {
		idx++
	}
	if idx == 0 || idx+1 >= len(trimmed) {
		return false
	}
	switch trimmed[idx] {
	case '.', ')':
		return trimmed[idx+1] == ' '
	default:
		return false
	}
}

func looksLikeNarrativeOutput(trimmed string, low string) bool {
	fields := strings.Fields(low)
	if len(fields) < 7 {
		return false
	}
	if !strings.ContainsAny(low, ".!?") {
		return false
	}
	if strings.Contains(trimmed, " -") {
		return false
	}
	if strings.ContainsAny(trimmed, "|&;$<>`") {
		return false
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, `\`) {
		return false
	}

	commonWords := map[string]struct{}{
		"the": {}, "this": {}, "that": {}, "is": {}, "are": {}, "was": {}, "were": {}, "for": {}, "with": {},
		"from": {}, "and": {}, "or": {}, "to": {}, "of": {}, "in": {}, "on": {}, "only": {}, "directly": {},
		"matches": {}, "request": {}, "command": {}, "candidates": {}, "operations": {}, "unrelated": {},
	}
	commonCount := 0
	for _, field := range fields {
		word := strings.Trim(field, `"'.,!?;:()[]{}<>`)
		if _, ok := commonWords[word]; ok {
			commonCount++
		}
	}
	return commonCount >= 2
}

func isLikelyToolName(token string) bool {
	switch token {
	case "npm", "pnpm", "yarn", "pip", "poetry", "go", "cargo", "aws", "terraform", "docker", "kubectl":
		return true
	default:
		return false
	}
}

func isLikelyCommandStarter(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '.', '/', '_', '~':
		return true
	default:
		return false
	}
}

func tokenIndex(command string, token string) int {
	if token == "" {
		return -1
	}
	start := 0
	for start <= len(command)-len(token) {
		idx := strings.Index(command[start:], token)
		if idx < 0 {
			return -1
		}
		idx += start
		beforeOK := idx == 0 || !isTokenChar(rune(command[idx-1]))
		afterPos := idx + len(token)
		afterOK := afterPos >= len(command) || !isTokenChar(rune(command[afterPos]))
		if beforeOK && afterOK {
			return idx
		}
		start = idx + 1
	}
	return -1
}

func isTokenChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isInternalCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return true
	}
	low := strings.ToLower(trimmed)
	if strings.Contains(low, "go run ./cmd/ew") || strings.Contains(low, "go run ./cmd/_ew") {
		return true
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return true
	}
	first := historyPrimaryCommandToken(fields)
	base := strings.ToLower(filepath.Base(first))
	return base == "ew" || base == "_ew"
}

func historyPrimaryCommandToken(fields []string) string {
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
		if historyIsEnvAssignmentToken(token) {
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
				if strings.HasPrefix(next, "-") || historyIsEnvAssignmentToken(next) {
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

func historyIsEnvAssignmentToken(token string) bool {
	if strings.HasPrefix(token, "-") {
		return false
	}
	eq := strings.IndexRune(token, '=')
	if eq <= 0 {
		return false
	}
	return strings.IndexAny(token[:eq], "/\\") == -1
}

func newHistoryScanner(f *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxHistoryLineBytes)
	return scanner
}
