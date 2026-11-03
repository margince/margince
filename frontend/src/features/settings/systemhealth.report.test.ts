/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it, vi } from "vitest";
import { HEALTH_AT, overdueFinding } from "./systemhealth.fixtures";
import { readHealthReport } from "./systemhealth.report";

afterEach(() => vi.unstubAllGlobals());
const privateField = "not-for-the-report";
const cases = [
  {
    source: "intake",
    response: {
      generated_at: HEALTH_AT,
      window_days: 7,
      private_field: privateField,
      units: [
        {
          unit: "sample",
          refused: 2,
          private_field: privateField,
          refusals: [
            { refusal: "addresses", refused: 2, private_field: privateField },
          ],
        },
      ],
    },
    expected: {
      generated_at: HEALTH_AT,
      window_days: 7,
      units: [
        {
          unit: "sample",
          refused: 2,
          refusals: [{ refusal: "addresses", refused: 2 }],
        },
      ],
    },
  },
  {
    source: "providers",
    response: {
      configured: ["anthropic"],
      private_field: privateField,
      providers: [
        {
          provider: "anthropic",
          health: "down",
          since: HEALTH_AT,
          private_field: privateField,
        },
      ],
    },
    expected: {
      configured: ["anthropic"],
      providers: [{ provider: "anthropic", health: "down", since: HEALTH_AT }],
    },
  },
  {
    source: "search",
    response: {
      configured_identity: "demo/new@1024",
      populated_identity: "demo/old@1024",
      status: "idle",
      updated_at: HEALTH_AT,
      reindex_needed: true,
      entities_pending: 42,
      private_field: privateField,
    },
    expected: {
      configured_identity: "demo/new@1024",
      populated_identity: "demo/old@1024",
      status: "idle",
      updated_at: HEALTH_AT,
      reindex_needed: true,
      entities_pending: 42,
    },
  },
];

it.each(cases)(
  "copies approved $source fields without future response additions",
  async ({ source, response, expected }) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify(response), {
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );
    const report = await readHealthReport(
      { ...overdueFinding, source },
      new AbortController().signal,
    );
    expect(JSON.parse(report)).toEqual(expected);
    expect(report).not.toContain(privateField);
  },
);

it("copies bounded routing evidence from the owner flags, without colleague names", async () => {
  const counts = {
    created: 12,
    not_mine: 4,
    rerouted: 0,
    dismissed: 0,
    rejected: 0,
    completed: 5,
    expired: 0,
    open: 3,
  };
  const tallies = [
    {
      kind: "promise_task",
      counts,
      not_mine_share: 1 / 3,
      small_sample: false,
      above_threshold: true,
    },
  ];
  const colleagues = Array.from({ length: 51 }, (_, index) => ({
    user_id: `member-${index}`,
    status: "active",
    display_name: privateField,
    tallies,
  }));
  const response = {
    generated_at: HEALTH_AT,
    days: 7,
    first_day: "2026-10-08",
    last_day: "2026-10-14",
    share_threshold: 0.1,
    small_sample_below: 10,
    kinds: [{ kind: "promise_task", records_not_mine: true }],
    unattributed: 0,
    colleagues: [
      ...colleagues,
      {
        user_id: "not-flagged",
        status: "active",
        display_name: privateField,
        tallies: [{ ...tallies[0], above_threshold: false }],
      },
    ],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(response), {
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
  const report = await readHealthReport(
    { ...overdueFinding, source: "automatic" },
    new AbortController().signal,
  );
  const copied = JSON.parse(report);
  expect(copied.total_flagged_colleagues).toBe(51);
  expect(copied.colleagues).toHaveLength(50);
  expect(copied.colleagues[0].tallies[0].counts).toEqual(counts);
  expect(report).not.toContain(privateField);
  expect(report).not.toContain("not-flagged");
});
