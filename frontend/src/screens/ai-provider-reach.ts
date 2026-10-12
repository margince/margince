// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { type AvailableModels, useAvailableModels } from "./ai-models";
import { boundProviders, type RoutingRead } from "./ai-routing-query";

// Which providers a binding editor may offer, and which of them still lack the
// key a call needs.

type KeyStatus = components["schemas"]["AiProviderKeyStatus"];

// The adapters an editor offers: those this installation can use, and the one
// the binding names now even if it no longer can — dropping that would erase
// the lane's own state from its own editor. A vendor with a key row is usable
// once a key is sealed; a keyless one (ollama, vllm) once the availability
// probe reached it; `fake` only where the routing document already binds it.
// While the key list has not arrived nothing is hidden; once it has, a keyless
// adapter stays out until its probe answers, since an option that appears late
// costs less than one offered and then refused.
export function reachableProviders(
  all: readonly string[],
  keys: readonly KeyStatus[] | undefined,
  current: string | undefined,
  probes: KeylessProbes = NO_PROBES,
  routing?: RoutingRead["routing"],
): readonly string[] {
  if (!keys) return all;
  const status = new Map(keys.map((k) => [k.provider, k]));
  return all.filter((provider) => {
    if (provider === current) return true;
    if (provider === "fake") {
      return (
        routing !== undefined && boundProviders(routing)?.has(provider) === true
      );
    }
    if (isKeyless(provider)) {
      const answer = probes.get(provider);
      return answer !== undefined && !answer.unavailable;
    }
    const entry = status.get(provider);
    return !entry || entry.configured || entry.optional;
  });
}

const KEYLESS_ADAPTERS = ["ollama", "vllm"] as const;

function isKeyless(provider: string): boolean {
  return KEYLESS_ADAPTERS.some((adapter) => adapter === provider);
}

type KeylessProbes = ReadonlyMap<string, AvailableModels | undefined>;
const NO_PROBES: KeylessProbes = new Map();

// What each keyless chat adapter answers when asked, for the lane being edited.
// Two queries that fire only while the editor is mounted, and not at all for
// the decision lane, whose adapters both take a key.
export function useKeylessProbes(
  lane: string,
  enabled: boolean,
): KeylessProbes {
  const ollama = useAvailableModels("ollama", lane, enabled);
  const vllm = useAvailableModels("vllm", lane, enabled);
  return new Map([
    ["ollama", ollama.data],
    ["vllm", vllm.data],
  ]);
}

// A provider that takes a key and holds none. Keyless adapters (no entry in
// the key list) and optional-key ones are never missing a key.
export function missingKey(
  provider: string,
  keys: readonly KeyStatus[] | undefined,
): boolean {
  const entry = keys?.find((k) => k.provider === provider);
  return entry !== undefined && !entry.configured && !entry.optional;
}
