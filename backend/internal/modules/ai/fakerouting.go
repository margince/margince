// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// FakeRoutingConfig keeps offline calls on the real router while deriving
// fake bindings from the task contract, so a new ladder cannot go unbound.
func FakeRoutingConfig() RoutingConfig {
	fake := ProviderConfig{Provider: ProviderFake}
	tiers := make(map[Tier]ProviderConfig)
	for _, task := range AllTasks() {
		for _, tier := range TaskLadder(task) {
			tiers[tier] = fake
		}
	}
	return RoutingConfig{
		Profile:    ProfileEUHosted,
		Tiers:      tiers,
		Embeddings: EmbeddingsConfig{ProviderConfig: fake},
	}
}
