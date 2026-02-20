package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ashwch/ew/internal/appdirs"
)

const storeFileName = "memory.json"

type Entry struct {
	Query      string  `json:"query"`
	Command    string  `json:"command"`
	Score      float64 `json:"score"`
	Uses       int     `json:"uses"`
	Successes  int     `json:"successes"`
	Failures   int     `json:"failures"`
	UpdatedAt  string  `json:"updated_at"`
	LastUsedAt string  `json:"last_used_at,omitempty"`
}

type Store struct {
	Entries []Entry `json:"entries"`
}

type Match struct {
	Query   string  `json:"query"`
	Command string  `json:"command"`
	Score   float64 `json:"score"`
	Uses    int     `json:"uses"`
	Exact   bool    `json:"exact"`
}

func Load() (Store, string, error) {
	path, err := appdirs.StateFilePath(storeFileName)
	if err != nil {
		return Store{}, "", err
	}
	bytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Store{}, path, nil
	}
	if err != nil {
		return Store{}, "", fmt.Errorf("could not read memory store: %w", err)
	}
	var store Store
	if err := json.Unmarshal(bytes, &store); err != nil {
		return Store{}, "", fmt.Errorf("could not parse memory store: %w", err)
	}
	store.normalize()
	return store, path, nil
}

func Save(path string, store Store) error {
	store.normalize()
	payload, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode memory store: %w", err)
	}
	if _, err := appdirs.EnsureStateDir(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, ".ew-memory-*.json")
	if err != nil {
		return fmt.Errorf("could not create temp memory file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := func() {
		_ = os.Remove(tempPath)
	}
	if _, err := tempFile.Write(payload); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("could not write temp memory file: %w", err)
	}
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("could not secure temp memory file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("could not close temp memory file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return fmt.Errorf("could not atomically replace memory file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("could not secure memory file: %w", err)
	}
	return nil
}

func (s *Store) normalize() {
	if s == nil {
		return
	}
	entries := make([]Entry, 0, len(s.Entries))
	seen := map[string]struct{}{}
	for _, entry := range s.Entries {
		entry.Query = strings.TrimSpace(entry.Query)
		entry.Command = strings.TrimSpace(entry.Command)
		if entry.Query == "" || entry.Command == "" {
			continue
		}
		if entry.Score < 0 {
			entry.Score = 0
		}
		key := normalize(entry.Query) + "|" + normalize(entry.Command)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Score == entries[j].Score {
			return entries[i].UpdatedAt > entries[j].UpdatedAt
		}
		return entries[i].Score > entries[j].Score
	})
	s.Entries = entries
}

func (s *Store) Remember(query, command string) error {
	return s.adjust(query, command, 24, true, false)
}

func (s *Store) Learn(query, command string, success bool) error {
	if success {
		return s.adjust(query, command, 3, true, false)
	}
	return s.adjust(query, command, -2, false, true)
}

func (s *Store) Promote(query, command string) error {
	return s.adjust(query, command, 6, true, false)
}

func (s *Store) Demote(query, command string) error {
	return s.adjust(query, command, -6, false, true)
}

func (s *Store) adjust(query, command string, delta float64, success bool, failure bool) error {
	query = strings.TrimSpace(query)
	command = strings.TrimSpace(command)
	if query == "" || command == "" {
		return fmt.Errorf("query and command are required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	idx := s.entryIndex(query, command)
	if idx < 0 {
		entry := Entry{
			Query:     query,
			Command:   command,
			Score:     clampScore(12 + delta),
			Uses:      1,
			UpdatedAt: now,
		}
		if success {
			entry.Successes = 1
			entry.LastUsedAt = now
		}
		if failure {
			entry.Failures = 1
		}
		s.Entries = append(s.Entries, entry)
		s.normalize()
		return nil
	}

	entry := s.Entries[idx]
	entry.Score = clampScore(entry.Score + delta)
	entry.Uses++
	entry.UpdatedAt = now
	if success {
		entry.Successes++
		entry.LastUsedAt = now
	}
	if failure {
		entry.Failures++
	}
	if entry.Score <= 0 {
		s.removeAt(idx)
		s.normalize()
		return nil
	}
	s.Entries[idx] = entry
	s.normalize()
	return nil
}

func (s *Store) ForgetQuery(query string) int {
	query = normalize(query)
	if query == "" {
		return 0
	}
	kept := make([]Entry, 0, len(s.Entries))
	removed := 0
	for _, entry := range s.Entries {
		if normalize(entry.Query) == query {
			removed++
			continue
		}
		kept = append(kept, entry)
	}
	s.Entries = kept
	s.normalize()
	return removed
}

func (s *Store) Top(limit int) []Match {
	if limit <= 0 {
		limit = 8
	}
	out := make([]Match, 0, min(limit, len(s.Entries)))
	for _, entry := range s.Entries {
		out = append(out, Match{
			Query:   entry.Query,
			Command: entry.Command,
			Score:   entry.Score,
			Uses:    entry.Uses,
			Exact:   false,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Store) Search(query string, limit int) []Match {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if limit <= 0 {
		limit = 8
	}
	qn := normalize(query)
	if qn == "" {
		return nil
	}
	qTokens := splitTokens(qn)

	matches := make([]Match, 0, len(s.Entries))
	for _, entry := range s.Entries {
		en := normalize(entry.Query)
		if en == "" {
			continue
		}
		base, exact := similarityScore(qn, qTokens, en)
		if base <= 0 {
			continue
		}
		score := base + (entry.Score * 0.7) + recencyBonus(entry.UpdatedAt)
		matches = append(matches, Match{
			Query:   entry.Query,
			Command: entry.Command,
			Score:   score,
			Uses:    entry.Uses,
			Exact:   exact,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			if matches[i].Exact == matches[j].Exact {
				return matches[i].Uses > matches[j].Uses
			}
			return matches[i].Exact
		}
		return matches[i].Score > matches[j].Score
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func (s *Store) entryIndex(query, command string) int {
	qn := normalize(query)
	cn := normalize(command)
	for idx, entry := range s.Entries {
		if normalize(entry.Query) == qn && normalize(entry.Command) == cn {
			return idx
		}
	}
	return -1
}

func (s *Store) removeAt(index int) {
	if index < 0 || index >= len(s.Entries) {
		return
	}
	s.Entries = append(s.Entries[:index], s.Entries[index+1:]...)
}

func normalize(input string) string {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	for strings.Contains(trimmed, "  ") {
		trimmed = strings.ReplaceAll(trimmed, "  ", " ")
	}
	return trimmed
}

func splitTokens(input string) []string {
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '-' || r == '_' || r == ':' || r == '/'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if len(token) < 2 {
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

func similarityScore(query string, qTokens []string, candidate string) (float64, bool) {
	if query == candidate {
		return 24, true
	}
	score := 0.0
	if strings.Contains(candidate, query) {
		score += 10
	}
	if strings.Contains(query, candidate) {
		score += 8
	}
	cTokens := splitTokens(candidate)
	if len(qTokens) > 0 && len(cTokens) > 0 {
		cSet := map[string]struct{}{}
		for _, token := range cTokens {
			cSet[token] = struct{}{}
		}
		shared := 0
		for _, token := range qTokens {
			if _, ok := cSet[token]; ok {
				shared++
			}
		}
		if shared > 0 {
			score += float64(shared) * 3.2
			coverage := float64(shared) / float64(len(qTokens))
			score += coverage * 5
		}
	}
	return score, false
}

func recencyBonus(updatedAt string) float64 {
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(updatedAt))
	if err != nil {
		return 0
	}
	age := time.Since(ts)
	switch {
	case age < 12*time.Hour:
		return 4
	case age < 3*24*time.Hour:
		return 2.5
	case age < 14*24*time.Hour:
		return 1
	default:
		return 0
	}
}

func clampScore(score float64) float64 {
	switch {
	case score < 0:
		return 0
	case score > 100:
		return 100
	default:
		return score
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
