// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The embeddings lane is billed and routed like a task but declared outside the
// task contract, so its name and summary are spelled here.
const (
	embeddingsDisplayName = "Search and retrieval"
	embeddingsSummary     = "Turns records and documents into vectors so search and document questions can find them."
)

// taskLabel is what a screen calls a task and what it says the task does.
func taskLabel(t Task) (name, summary string) {
	if t == TaskEmbeddings {
		return embeddingsDisplayName, embeddingsSummary
	}
	return DisplayName(t), Summary(t)
}
