package intelligence

import (
	"regexp"
	"sort"
	"strings"
)

var familyWords = regexp.MustCompile(`[a-z]+`)

// CanonicalID includes explicit assignments even before evidence is matched.
func (m *Manager) CanonicalID(publicID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.physical[publicID]
}

// FuzzyBenchmarkCandidates is only a shortlist, never identity evidence.
// Splitting letters from versions makes gemma4:e2b find every Gemma variant.
func FuzzyBenchmarkCandidates(upstream string, observations []Observation) []string {
	name := strings.ToLower(upstream)
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	words := familyWords.FindAllString(name, -1)
	ignored := map[string]bool{"instruct": true, "chat": true, "latest": true, "preview": true, "free": true, "model": true, "quantized": true, "thinking": true, "text": true}
	family := ""
	for _, word := range words {
		if len(word) >= 3 && !ignored[word] {
			family = word
			break
		}
	}
	if family == "" {
		return nil
	}
	seen := map[string]bool{}
	for _, observation := range observations {
		if strings.Contains(strings.ToLower(observation.ExternalModelID), family) {
			seen[observation.ExternalModelID] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
