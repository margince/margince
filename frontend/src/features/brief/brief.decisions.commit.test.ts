/** @vitest-environment happy-dom */
import { afterEach, describe, expect, it, vi } from "vitest";
import { deckItems } from "../../lib/brief.decisions.items";
import { proposal } from "../../testing/brief.fixtures";
import { commitTray } from "./brief.decisions.commit";

function answer(path: string): Response {
  const body = path.includes("approval-bundles")
    ? {
        bundle_id: "b1",
        data: [
          { approval: { id: "m1", awaiting_agent: true }, outcome: "decided" },
          { approval: { id: "m2" }, outcome: "effect_failed" },
          { approval: { id: "m3" }, outcome: "decided" },
        ],
      }
    : { id: "ap-1", status: "approved", awaiting_agent: true };
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => vi.unstubAllGlobals());

describe("what a sent tray reports", () => {
  it("counts an approve that only waits for its agent", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => answer(new URL(input.url).pathname)),
    );
    const items = deckItems([proposal("ap-1", "Read the page")]);

    const result = await commitTray({
      staged: [{ id: items[0].id, verdict: "accept" }],
      items,
    });

    expect(result.awaitingAgent).toBe(1);
    expect(result.effectFailed).toBe(0);
  });

  it("carries each bundle member's outcome", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => answer(new URL(input.url).pathname)),
    );
    const items = deckItems([
      proposal("m1", "One", { bundle_id: "b1" }),
      proposal("m2", "Two", { bundle_id: "b1" }),
      proposal("m3", "Three", { bundle_id: "b1" }),
    ]);

    const result = await commitTray({
      staged: [{ id: items[0].id, verdict: "accept" }],
      items,
    });

    expect(result.awaitingAgent).toBe(1);
    expect(result.effectFailed).toBe(1);
  });
});
