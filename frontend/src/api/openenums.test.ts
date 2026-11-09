// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./client";

// A response enum is open in the contract, so a server newer than this build
// can send a value the generated types do not list. The client must hand the
// response over whole rather than fail it.

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("the api client reads a response enum value it does not know", () => {
  it("returns the whole response, unknown values kept", async () => {
    const body = {
      lanes_omitted: ["a_lane_from_a_later_release"],
      needs_you: [
        {
          source: "a_source_from_a_later_release",
          actions: ["an_action_from_a_later_release"],
        },
      ],
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify(body), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );

    const { data, error } = await api.GET("/attention");

    expect(error).toBeUndefined();
    expect(data?.lanes_omitted).toEqual(["a_lane_from_a_later_release"]);
    expect(data?.needs_you[0]?.source).toBe("a_source_from_a_later_release");
    expect(data?.needs_you[0]?.actions).toEqual([
      "an_action_from_a_later_release",
    ]);
  });
});
