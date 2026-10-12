import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { modelText } from "./agentrail.model";
import type { AiCall } from "./agentrail-reads";

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

function call(kind: AiCall["kind"], provider: string, served: string): AiCall {
  return {
    id: `${kind}-${served}`,
    occurred_at: "2026-07-20T10:00:00Z",
    kind,
    task: "capture_classify",
    tier: "cheap_cloud",
    provider,
    model_id: "configured",
    served_model: served,
    calls_attempted: 1,
    tokens_in: 10,
    tokens_out: 5,
    reasoning_tokens: 0,
    cached_tokens: 0,
    latency_ms: 400,
    cache_hit: false,
    degraded: false,
    has_payload: false,
    decision_attempted: false,
  };
}

describe("modelText — which model the agent panel names", () => {
  it("names the model that answers over a newer embedding", () => {
    const calls = [
      call("embedding", "openai", "text-embedding"),
      call("completion", "gemini", "served"),
    ];
    expect(modelText({ allowed: true, calls }, t)).toBe("gemini/served");
  });

  it("names the newest model that answered, past an embedding and a failure", () => {
    const failed = {
      ...call("completion", "anthropic", "failed"),
      error_sentinel: "provider_unavailable",
    };
    const calls = [
      failed,
      call("embedding", "openai", "text-embedding"),
      call("completion", "gemini", "newer"),
      call("completion", "openai", "older"),
    ];
    expect(modelText({ allowed: true, calls }, t)).toBe("gemini/newer");
  });

  it("names no model when every call in the window failed", () => {
    const failed = {
      ...call("completion", "anthropic", "failed"),
      error_sentinel: "provider_unavailable",
    };
    expect(modelText({ allowed: true, calls: [failed] }, t)).toBe(
      translate("en", "agent.fact.noCalls"),
    );
  });

  it("names an embedding as the search index when that is all there is", () => {
    const calls = [call("embedding", "openai", "text-embedding")];
    expect(modelText({ allowed: true, calls }, t)).toBe(
      "Search index: openai/text-embedding",
    );
  });
});
