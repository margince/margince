// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package ai owns the model runtime behind the shared/ports/model seam. It
// holds the provider adapters, which providerRegistry lists. It holds the
// SelectBrain factory that turns config into a Client, and the
// credential SecretStripper that runs over every outbound payload.
//
// Model choice is config, not architecture (ADR-0020): vendor names
// appear only here and in ai-routing.yaml — callers name capability
// tiers. The stripper is hygiene, not privacy: privacy is the location
// ladder (A8 revised), so nothing here pseudonymizes PII.
//
// Tables owned: docs/reference/entity-model/ai.md.
//
// ai_usage is per-workspace metering and feeds the budget guardrail.
// voice_profile and voice_corpus_source (Voice DNA) are model-adjacent assets,
// so they live beside the runtime that consumes them. ai_call is one row per
// rung a logical call walked; ai_call_payload is the opt-in post-stripper
// content that privacy's retention and Art. 17 erasure age out and purge.
// ai_call_config is the hash-keyed, append-only config-snapshot
// dimension every ai_call row points at; it carries no workspace_id —
// a task contract, routing yaml, and prompt version are build facts,
// not tenant data.
// ai_model_rate is the effective-dated rate sheet the read-side pricer prices
// calls against. It carries no workspace_id column: a published provider price
// is the same number for every workspace, and pricing on READ is what lets a
// corrected rate reprice history instead of contradicting it.
// Imports shared + platform only; never a sibling module.
package ai
