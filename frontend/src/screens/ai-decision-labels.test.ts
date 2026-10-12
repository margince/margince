// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import {
  callCodeName,
  callCodeTone,
  gaveUpLabel,
  isKnownCallCode,
  KNOWN_CALL_CODES,
  TIER_ORDER,
  tierLabel,
  tierRank,
} from "./ai-decision-labels";

const t = (key: keyof typeof en) => en[key];

describe("tierLabel", () => {
  it("names every ladder tier and the lanes beside it", () => {
    for (const tier of [
      ...TIER_ORDER,
      "embeddings",
      "embed",
      "decisions",
      "decide",
    ]) {
      expect(tierLabel(tier, t)).not.toBe(tier);
    }
    expect(tierLabel("cheap_cloud", t)).toBe("Everyday cloud");
    expect(tierLabel("embed", t)).toBe(tierLabel("embeddings", t));
    expect(tierLabel("decisions", t)).toBe(tierLabel("decide", t));
  });

  it("reads a tier nobody named as its own key", () => {
    expect(tierLabel("nightly_batch", t)).toBe("nightly_batch");
  });
});

describe("TIER_ORDER", () => {
  it("ranks every tier once, cheapest first, and a tier off it last", () => {
    expect(new Set(TIER_ORDER).size).toBe(TIER_ORDER.length);
    expect(TIER_ORDER.map(tierRank)).toEqual(TIER_ORDER.map((_, i) => i));
    expect(tierRank("embeddings")).toBe(TIER_ORDER.length);
  });
});

// The codes are the server's, read from the Go that files them. A code added
// there without words here fails this test rather than reading "Failed".
function filedCodes(): { codes: string[]; returns: number } {
  const here = dirname(fileURLToPath(import.meta.url));
  const ai = join(here, "../../../backend/internal/modules/ai");
  const constants = new Map<string, string>();
  for (const file of ["callsentinels.go", "callstore.go"]) {
    const source = readFileSync(join(ai, file), "utf8");
    for (const [, name, value] of source.matchAll(
      /\b(sentinel[A-Z]\w*)\s*=\s*"([a-z_]+)"/g,
    )) {
      constants.set(name, value);
    }
  }
  const store = readFileSync(join(ai, "callstore.go"), "utf8");
  const start = store.indexOf("func classifyError(");
  const body = store.slice(start, store.indexOf("\n}\n", start));
  const returns = [...body.matchAll(/\breturn\s+(\S+)/g)].map(([, value]) =>
    value.startsWith('"')
      ? value.slice(1, -1)
      : (constants.get(value) ?? value),
  );
  return { codes: returns.filter(Boolean), returns: returns.length };
}

describe("call codes", () => {
  it("has words for every code the server files on a call", () => {
    const { codes, returns } = filedCodes();
    expect(codes.length).toBe(returns - 1);
    expect(codes).toEqual(
      expect.arrayContaining([
        "budget_deferred",
        "metering_failed",
        "budget_unavailable",
        "request_failed",
        "timeout",
        "provider_quota",
        "provider_throttled",
        "provider_refused",
        "output_withheld",
        "request_rejected",
        "output_rejected",
        "provider_error",
      ]),
    );
    for (const code of codes) {
      expect(KNOWN_CALL_CODES, code).toContain(code);
      expect(callCodeName(code, t), code).toMatch(/\p{L}/u);
      expect(callCodeName(code, t)).not.toBe(t("aicalls.outcome.failed"));
      expect(gaveUpLabel(code, t), code).toMatch(/\p{L}/u);
      expect(gaveUpLabel(code, t)).not.toBe(code);
    }
  });

  it("warns, not fails, on a call that was served or put off", () => {
    expect(callCodeTone("metering_failed")).toBe("warning");
    expect(callCodeTone("budget_deferred")).toBe("warning");
    expect(callCodeTone("provider_error")).toBe("danger");
  });

  it("quotes a code nobody named rather than calling it a known failure", () => {
    expect(isKnownCallCode("provider_melted")).toBe(false);
    expect(gaveUpLabel("provider_melted", t)).toBe("provider_melted");
  });
});
