package provider

// markDecisionModel records protocol evidence without inferring support from a
// model's name. Decision-only models must never enter a chat routing pool.
func markDecisionModel(model *Model) {
	model.Decision = true
	model.Discovery = &ModelDiscovery{Source: "provider_decision_catalog", Endpoints: map[string]bool{
		"systemone": true, "chat_completions": false, "completions": false, "responses": false, "embeddings": false,
	}}
}
