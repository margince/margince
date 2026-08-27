/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { scanStates } from "./organizations";

type SiteReadReport = components["schemas"]["SiteReadReport"];

// The ladder may only report positions the engine actually holds. A rung that
// advanced on a timer would be a progress bar wearing a checklist, and the
// reason this reads `phase` rather than elapsed time is that a read can sit in
// one stage for minutes without anything being wrong.

function report(over: Partial<SiteReadReport>): SiteReadReport {
  return {
    read_id: "r-1",
    organization_id: "o-1",
    seed_url: "https://example.test",
    status: "running",
    status_code: null,
    status_detail: null,
    next_attempt_at: null,
    pages: [],
    skipped: [],
    proposal_ids: [],
    created_at: "2026-08-26T08:00:00Z",
    ...over,
  } satisfies SiteReadReport;
}

describe("scanStates", () => {
  it("waits on both stages before the read has started", () => {
    expect(scanStates(report({ status: "queued" }))).toEqual({
      crawling: "queued",
      extracting: "queued",
    });
  });

  it("waits the same way while a read is held for budget", () => {
    // Deferred is still a read in flight — it has simply not been given the
    // budget to start — so it waits rather than showing progress it has not
    // made.
    expect(scanStates(report({ status: "deferred" }))).toEqual({
      crawling: "queued",
      extracting: "queued",
    });
  });

  it("treats a read that is extracting as having finished crawling", () => {
    // The engine's own ordering, not a guess about elapsed time: it cannot
    // extract from pages it has not fetched.
    expect(
      scanStates(report({ status: "running", phase: "extracting" })),
    ).toEqual({ crawling: "done", extracting: "running" });
  });

  it("reads a running read with no phase yet as crawling", () => {
    // `phase` is null the moment a read goes terminal, and briefly before the
    // worker has claimed a stage. Neither is a reason to draw an empty ladder.
    expect(scanStates(report({ status: "running", phase: null }))).toEqual({
      crawling: "running",
      extracting: "queued",
    });
  });
});
