// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it, vi } from "vitest";
import { readDraft } from "./composesaveddraftcalls";

const ANCHOR = { type: "contact", id: "p-1" } as const;

afterEach(() => {
  vi.unstubAllGlobals();
});

function answer(response: Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response),
  );
}

it("reads no draft here as none", async () => {
  answer(new Response(null, { status: 204 }));

  expect(await readDraft(ANCHOR)).toBeNull();
});

// The contract makes a hidden anchor and no draft one answer.
it("reads an anchor the reader can no longer see as none", async () => {
  answer(
    new Response(JSON.stringify({ code: "not_found" }), {
      status: 404,
      headers: { "Content-Type": "application/problem+json" },
    }),
  );

  expect(await readDraft(ANCHOR)).toBeNull();
});

it("reports any other failure", async () => {
  answer(
    new Response(JSON.stringify({ code: "internal" }), {
      status: 500,
      headers: { "Content-Type": "application/problem+json" },
    }),
  );

  await expect(readDraft(ANCHOR)).rejects.toThrow();
});
