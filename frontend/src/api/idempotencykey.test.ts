// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
import { describe, expect, it } from "vitest";
import { intentKeys } from "./idempotencykey";

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
