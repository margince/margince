// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";

// What OpenRouter will be asked for, one plain sentence per key of the merged
// request the server's preview returns. The merge is the server's: this file
// only words it and says where each key came from.

export type Source = "default" | "connection" | "tier" | "task";

export type SummaryLine = Readonly<{
  key: string;
  value: string;
  source: Source;
  sentence: Readonly<{ key: MessageKey; params?: Record<string, string> }>;
}>;

/** The keys the connection owns: they come from there whatever a tier says. */
export const CONNECTION_KEYS = [
  "only",
  "ignore",
  "allow_fallbacks",
  "zdr",
  "data_collection",
  "enforce_distillable_text",
];

type Json = string | number | boolean | null | Json[] | { [key: string]: Json };

function isObject(value: unknown): value is Record<string, Json> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function list(value: Json): string {
  return (Array.isArray(value) ? value : [value]).map(String).join(", ");
}

function perPercentile(value: Json, unit: string): string {
  if (typeof value === "number") return `${value}${unit}`;
  if (!isObject(value)) return String(value);
  return Object.entries(value)
    .map(([pct, v]) => `${v}${unit} at ${pct}`)
    .join(", ");
}

type Sentence = SummaryLine["sentence"];

const flag =
  (on: MessageKey, off: MessageKey) =>
  (value: Json): Sentence => ({ key: value ? on : off });
const hostsSay =
  (key: MessageKey) =>
  (value: Json): Sentence => ({ key, params: { hosts: list(value) } });

// One sentence per key OpenRouter takes; a key not here reads as itself.
const SAY: Readonly<Record<string, (value: Json) => Sentence>> = {
  "provider.sort": (value) => ({
    key:
      isObject(value) && value.partition === "none"
        ? "aiServing.say.sortAcross"
        : "aiServing.say.sort",
    params: { by: isObject(value) ? String(value.by) : String(value) },
  }),
  "provider.quantizations": (value) => ({
    key: "aiServing.say.quantizations",
    params: { levels: list(value) },
  }),
  "provider.require_parameters": flag(
    "aiServing.say.requireParameters",
    "aiServing.say.requireParametersOff",
  ),
  "provider.zdr": () => ({ key: "aiServing.say.zdr" }),
  "provider.data_collection": (value) => ({
    key:
      value === "deny"
        ? "aiServing.say.denyCollection"
        : "aiServing.say.allowCollection",
  }),
  "provider.enforce_distillable_text": () => ({ key: "aiServing.say.distill" }),
  "provider.only": hostsSay("aiServing.say.only"),
  "provider.ignore": hostsSay("aiServing.say.ignore"),
  "provider.order": hostsSay("aiServing.say.order"),
  "provider.allow_fallbacks": flag(
    "aiServing.say.fallbacks",
    "aiServing.say.noFallbacks",
  ),
  "provider.max_price": (value) => ({
    key: "aiServing.say.maxPrice",
    params: {
      prices: isObject(value)
        ? Object.entries(value)
            .map(([k, v]) => `$${v} ${k}`)
            .join(", ")
        : String(value),
    },
  }),
  "provider.preferred_max_latency": (value) => ({
    key: "aiServing.say.maxLatency",
    params: { latency: perPercentile(value, " s") },
  }),
  "provider.preferred_min_throughput": (value) => ({
    key: "aiServing.say.minThroughput",
    params: { throughput: perPercentile(value, "") },
  }),
  "reasoning.effort": (value) => ({
    key: "aiServing.say.effort",
    params: { effort: String(value) },
  }),
  "reasoning.max_tokens": (value) => ({
    key: "aiServing.say.maxTokens",
    params: { tokens: String(value) },
  }),
  "reasoning.exclude": flag("aiServing.say.exclude", "aiServing.say.include"),
  "reasoning.enabled": flag("aiServing.say.thinkOn", "aiServing.say.thinkOff"),
};

function sentenceFor(key: string, value: Json): Sentence {
  return (
    SAY[key]?.(value) ?? {
      key: "aiServing.say.raw",
      params: { key, value: JSON.stringify(value) },
    }
  );
}

function sourceOf(
  block: string,
  key: string,
  own: Record<string, Json>,
): Source {
  if (block === "provider" && CONNECTION_KEYS.includes(key))
    return "connection";
  return key in own ? "tier" : "default";
}

function blockLines(
  block: "provider" | "reasoning",
  effective: unknown,
  written: Record<string, Json>,
): SummaryLine[] {
  const merged = isObject(effective) ? effective[block] : undefined;
  if (!isObject(merged)) return [];
  const own = isObject(written[block]) ? written[block] : {};
  return Object.entries(merged).map(([key, value]) => ({
    key: `${block}.${key}`,
    value: JSON.stringify(value),
    source: sourceOf(block, key, own),
    sentence: sentenceFor(`${block}.${key}`, value),
  }));
}

/**
 * One line per key of `effective`, the merged request, with its source: the
 * connection for its own keys, this tier for a key it wrote, else the shipped
 * default. A tier that names no effort gets a closing line saying each task's
 * thinking level decides it.
 */
export function summarize(
  effective: unknown,
  written: unknown,
  thinks: boolean,
): SummaryLine[] {
  const mine = isObject(written) ? written : {};
  const lines = [
    ...blockLines("provider", effective, mine),
    ...blockLines("reasoning", effective, mine),
  ];
  const named = lines.some((line) => line.key === "reasoning.effort");
  if (thinks && !named) {
    lines.push({
      key: "reasoning.effort",
      value: "",
      source: "task",
      sentence: { key: "aiServing.say.taskEffort" },
    });
  }
  return lines;
}
