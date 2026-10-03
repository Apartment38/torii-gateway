package intelligence

import "sort"

// TableModel is a management-only summary. Unmatched benchmark models are
// inspectable here, but never enter the live registry or influence routing.
type TableModel struct {
	Model
	DisplayName string   `json:"display_name"`
	Match       string   `json:"match"`
	ExternalIDs []string `json:"external_ids"`
}

// TableModels aggregates the entire cached dataset before the console paginates
// it. Canonical aliases use the existing matcher; otherwise only identical
// normalized external IDs are grouped. Dates, effort and version suffixes are
// preserved, and ambiguous identities stay explicitly marked as ambiguous.
func (m *Manager) TableModels(snapshot Snapshot) map[string]TableModel {
	rows := map[string]TableModel{}
	for id, model := range snapshot.Models {
		external := map[string]bool{}
		for _, observation := range model.Observations {
			external[observation.ExternalModelID] = true
		}
		model.Observations = nil
		rows[id] = TableModel{Model: model, DisplayName: id, Match: "matched", ExternalIDs: tableKeys(external)}
	}
	ambiguous := map[string]bool{}
	for _, entry := range snapshot.Unmatched {
		if entry.Reason == "ambiguous" {
			ambiguous[entry.Source+"\x00"+entry.ExternalID] = true
		}
	}
	groups := map[string][]Observation{}
	names := map[string]string{}
	for _, source := range tableKeys(snapshot.Sources) {
		state := snapshot.Sources[source]
		if !state.Enabled {
			continue
		}
		for _, observation := range state.Observations {
			if observation.ModelID != "" {
				continue
			}
			id := "external/" + NormalizeIdentity(observation.ExternalModelID)
			groups[id] = append(groups[id], observation)
			if names[id] == "" {
				names[id] = observation.ExternalModelID
			}
		}
	}
	for _, id := range tableKeys(groups) {
		observations := groups[id]
		model := Aggregate(observations, m.options.Aggregation, m.now().UTC(), m.options.HalfLife, m.options.MaxAge, m.options.StaleAfter)
		model.ID, model.ObservationCount, model.Observations = id, len(observations), nil
		sources, external := map[string]bool{}, map[string]bool{}
		match := "unmatched"
		for _, observation := range observations {
			sources[observation.Source], external[observation.ExternalModelID] = true, true
			if ambiguous[observation.Source+"\x00"+observation.ExternalModelID] {
				match = "ambiguous"
			}
		}
		model.Sources = tableKeys(sources)
		// Keep display-only keys disjoint from canonical provider IDs.
		for {
			if _, exists := rows[model.ID]; !exists {
				break
			}
			model.ID = "external/" + model.ID
		}
		rows[model.ID] = TableModel{Model: model, DisplayName: names[id], Match: match, ExternalIDs: tableKeys(external)}
	}
	return rows
}

func tableKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
