// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

// One binding inside the routing document, addressed on its own.
//
// The wire has ONE document and one ETag, so every edit is a whole-document
// write. What an editor owns is a slice of it: the tier, the embedder, or the
// decision model it opened on. Saving puts that slice back onto the LATEST
// document, so an edit to one lane never carries a stale copy of another.

type Routing = components["schemas"]["AiRouting"];
type TierBinding = components["schemas"]["AiTierBinding"];
type EmbeddingsBinding = components["schemas"]["AiEmbeddingsBinding"];
type DecisionsBinding = components["schemas"]["AiDecisionsBinding"];

export type RoutingSlice =
  | { kind: "tier"; tier: string }
  | { kind: "embeddings" }
  | { kind: "decisions" };

/** One slice and what it binds. Only the decision model may bind nothing. */
export type SliceValue =
  | { kind: "tier"; tier: string; binding: TierBinding }
  | { kind: "embeddings"; binding: EmbeddingsBinding }
  | { kind: "decisions"; binding: DecisionsBinding | undefined };

/**
 * The value one slice holds. A tier the document does not bind reads as an
 * empty binding rather than failing: the editor can only have been opened on a
 * tier the document named, and a colleague who since dropped it has changed the
 * slice, which the conflict check then reports.
 */
export function sliceOf(routing: Routing, slice: RoutingSlice): SliceValue {
  switch (slice.kind) {
    case "tier":
      return {
        kind: "tier",
        tier: slice.tier,
        binding: routing.tiers[slice.tier] ?? { provider: "", model: "" },
      };
    case "embeddings":
      return { kind: "embeddings", binding: routing.embeddings };
    case "decisions":
      return { kind: "decisions", binding: routing.decisions };
  }
}

/**
 * The document with one slice replaced. An unbound decision model is deleted
 * rather than left holding undefined: absent is what the server reads as "no
 * decision model".
 */
export function withSlice(routing: Routing, value: SliceValue): Routing {
  switch (value.kind) {
    case "tier":
      return {
        ...routing,
        tiers: { ...routing.tiers, [value.tier]: value.binding },
      };
    case "embeddings":
      return { ...routing, embeddings: value.binding };
    case "decisions": {
      const next: Routing = { ...routing, decisions: value.binding };
      if (value.binding === undefined) {
        delete next.decisions;
      }
      return next;
    }
  }
}

/**
 * Whether two readings of one slice are the same binding. Key order and an
 * explicitly undefined field are not differences: the server answers fields in
 * its own order and omits what is unset.
 */
export function sameSlice(a: SliceValue, b: SliceValue): boolean {
  return canonical(a.binding) === canonical(b.binding);
}

function canonical(binding: object | undefined): string {
  if (binding === undefined) return "";
  return JSON.stringify(
    Object.entries(binding)
      .filter(([, value]) => value !== undefined)
      .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)),
  );
}
