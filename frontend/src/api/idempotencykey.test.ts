// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
import { describe, expect, it, vi } from "vitest";
import { intentKeys } from "./idempotencykey";
import { keyedIntents } from "./intentkeys";

const answering = (status: number, code?: string) => (key: string) =>
  Promise.resolve({
    key,
    response: new Response(null, { status }),
    error: code === undefined ? undefined : { code },
  });

describe("intentKeys", () => {
  it("reuses an open request's key after another request in between", async () => {
    const keyed = intentKeys();
    const a1 = await keyed({ review: "A" }, answering(500));
    const b = await keyed({ review: "B" }, answering(500));
    const a2 = await keyed({ review: "A" }, answering(201));
    expect(a2.key).toBe(a1.key);
    expect(b.key).not.toBe(a1.key);
  });

  it("drops a key once its request is answered", async () => {
    const keyed = intentKeys();
    const sent = await keyed({ review: "A" }, answering(201));
    const refused = await keyed(
      { review: "A" },
      answering(409, "consent_not_granted"),
    );
    const again = await keyed({ review: "A" }, answering(201));
    expect(refused.key).not.toBe(sent.key);
    expect(again.key).not.toBe(refused.key);
  });

  it("keeps the key while the first request is still running", async () => {
    const keyed = intentKeys();
    const first = await keyed(
      { review: "A" },
      answering(409, "idempotency_key_in_progress"),
    );
    const retry = await keyed({ review: "A" }, answering(201));
    expect(retry.key).toBe(first.key);
  });

  it("keeps the key when the request never got an answer", async () => {
    const keyed = intentKeys();
    let lost = "";
    await expect(
      keyed({ review: "A" }, (key) => {
        lost = key;
        return Promise.reject(new TypeError("Failed to fetch"));
      }),
    ).rejects.toThrow();
    const retry = await keyed({ review: "A" }, answering(201));
    expect(retry.key).toBe(lost);
  });

  it("holds at most its limit of open requests, oldest dropped first", async () => {
    const keyed = intentKeys(2);
    const a = await keyed("A", answering(500));
    await keyed("B", answering(500));
    await keyed("C", answering(500));
    expect((await keyed("A", answering(500))).key).not.toBe(a.key);
  });
});

describe("keyedIntents", () => {
  const spent = (r: { done: boolean }) => r.done;

  it("keeps an unsettled intent's key and drops a settled one", async () => {
    const keyed = keyedIntents(spent);
    const seen: string[] = [];
    const call = (done: boolean) => async (key: string) => {
      seen.push(key);
      return { done };
    };
    await keyed("a", call(false));
    await keyed("a", call(true));
    await keyed("a", call(true));
    expect(seen[1]).toBe(seen[0]);
    expect(seen[2]).not.toBe(seen[0]);
  });

  it("lets a late settle leave a newer key of the same intent alone", async () => {
    const keyed = keyedIntents(spent);
    const seen: string[] = [];
    let release: (v: { done: boolean }) => void = () => undefined;
    const slow = keyed("a", (key) => {
      seen.push(key);
      return new Promise((resolve) => {
        release = resolve;
      });
    });
    await keyed("a", async (key) => {
      seen.push(key);
      return { done: true };
    });
    await keyed("a", async (key) => {
      seen.push(key);
      return { done: false };
    });
    release({ done: true });
    await slow;
    await keyed("a", async (key) => {
      seen.push(key);
      return { done: true };
    });
    expect(seen[0]).toBe(seen[1]);
    expect(seen[2]).not.toBe(seen[0]);
    expect(seen[3]).toBe(seen[2]);
  });

  it("holds at most the limit of open intents, oldest dropped first", async () => {
    const keyed = keyedIntents(spent, 2);
    const seen = new Map<string, string[]>();
    const open = (intent: string) =>
      keyed(intent, async (key) => {
        seen.set(intent, [...(seen.get(intent) ?? []), key]);
        return { done: false };
      });
    await open("a");
    await open("b");
    await open("c");
    await open("a");
    expect(seen.get("a")?.[1]).not.toBe(seen.get("a")?.[0]);
    await open("c");
    expect(seen.get("c")?.[1]).toBe(seen.get("c")?.[0]);
  });
});

describe("randomKey", () => {
  it("makes a version 4 UUID where randomUUID is absent, as in a sandboxed card frame", async () => {
    vi.stubGlobal("crypto", {
      getRandomValues: crypto.getRandomValues.bind(crypto),
    });
    try {
      const keyed = keyedIntents((r: { done: boolean }) => r.done);
      const seen: string[] = [];
      await keyed("a", async (key) => {
        seen.push(key);
        return { done: true };
      });
      await keyed("a", async (key) => {
        seen.push(key);
        return { done: true };
      });
      for (const key of seen) {
        expect(key).toMatch(
          /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
        );
      }
      expect(seen[0]).not.toBe(seen[1]);
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
