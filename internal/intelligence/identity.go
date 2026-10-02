package intelligence

import (
	"sort"
	"strings"

	"github.com/Apartment38/torii-gateway/internal/catalog"
)

type Matcher struct {
	aliases   map[string]map[string]bool
	scoped    map[string]map[string]bool
	providers map[string][]string
	physical  map[string]string
}

// NormalizeIdentity deliberately preserves dates, versions, punctuation,
// quantization and effort suffixes. It never fuzzy-matches a model family.
func NormalizeIdentity(id string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(strings.TrimSpace(id), "_", " ")), "-"))
}

func NewMatcher(models []catalog.Descriptor, identities []Identity) *Matcher {
	m := &Matcher{aliases: map[string]map[string]bool{}, scoped: map[string]map[string]bool{}, providers: map[string][]string{}, physical: map[string]string{}}
	assignments := map[string]map[string]bool{}
	for _, identity := range identities {
		for _, id := range identity.ProviderIDs {
			addAlias(assignments, id, identity.CanonicalID)
		}
	}
	for _, model := range models {
		canonical := model.PublicID
		if assigned := assignments[model.PublicID]; len(assigned) == 1 {
			for id := range assigned {
				canonical = id
			}
		} else if len(assigned) > 1 {
			// Conflicting explicit physical assignments stay ambiguous too.
			for id := range assigned {
				addAlias(m.aliases, NormalizeIdentity(model.PublicID), id)
			}
			continue
		}
		m.physical[model.PublicID] = canonical
		m.providers[canonical] = append(m.providers[canonical], model.PublicID)
		for _, alias := range []string{canonical, model.PublicID, model.UpstreamID} {
			addAlias(m.aliases, NormalizeIdentity(alias), canonical)
		}
	}
	for _, identity := range identities {
		// Do not accept an alias for a model the live catalog doesn't know.
		if len(m.providers[identity.CanonicalID]) == 0 {
			continue
		}
		for _, alias := range append(identity.Aliases, identity.CanonicalID) {
			addAlias(m.aliases, NormalizeIdentity(alias), identity.CanonicalID)
		}
		for source, ids := range identity.SourceIDs {
			for _, id := range ids {
				addAlias(m.scoped, source+"\x00"+NormalizeIdentity(id), identity.CanonicalID)
			}
		}
	}
	for id := range m.providers {
		sort.Strings(m.providers[id])
	}
	return m
}

func addAlias(index map[string]map[string]bool, alias, canonical string) {
	if alias == "" || canonical == "" {
		return
	}
	if index[alias] == nil {
		index[alias] = map[string]bool{}
	}
	index[alias][canonical] = true
}

func (m *Matcher) Match(source, external string) (string, []string) {
	ids := m.scoped[source+"\x00"+NormalizeIdentity(external)]
	if len(ids) == 0 {
		ids = m.aliases[NormalizeIdentity(external)]
	}
	candidates := make([]string, 0, len(ids))
	for id := range ids {
		candidates = append(candidates, id)
	}
	sort.Strings(candidates)
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return "", candidates
}
