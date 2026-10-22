// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { translate, translatePlural } from "../../i18n";
import {
  intakeReading,
  jobsReading,
  oldestStamp,
  providersReading,
  readHealth,
  searchIndexReading,
} from "./systemhealthreadings";

const words = {
  t: (key: Parameters<typeof translate>[1], params?: Record<string, string>) =>
    translate("en", key, params),
  plural: (
    base: Parameters<typeof translatePlural>[1],
    count: number,
    params?: Record<string, string>,
  ) => translatePlural("en", base, count, params),
  locale: "en" as const,
};

type JobHealth = components["schemas"]["JobHealth"];
type Kind = JobHealth["kinds"][number];

const kind = (counts: Partial<Kind>): Kind => ({
  kind: "webhook_retry",
  queue: "default",
  fleet_wide: false,
  waiting: 0,
  running: 0,
  retrying: 0,
  dead: 0,
  dead_recent: 0,
  oldest_waiting_age_seconds: null,
  ...counts,
});

const jobs = (kinds: Kind[], failures = 0): JobHealth => ({
  generated_at: "2026-10-14T09:00:00Z",
  dead_window_hours: 24,
  kinds,
  recent_failures: Array.from({ length: failures }, () => ({
    kind: "webhook_retry",
    state: "cancelled" as const,
    reason: "Stopped.",
    attempt: 1,
    max_attempts: 5,
    failed_at: "2026-10-10T09:00:00Z",
  })),
});

describe("readHealth", () => {
  const settled = { data: "report", isError: false, errorUpdateCount: 0 };

  it("waits for the session before it calls a reading withheld", () => {
    const probe = { canSee: false, probing: true, query: settled };
    expect(readHealth(probe, words.t, () => fail()).card?.value).toBe(
      "Checking…",
    );
  });

  it("gives a withheld reading no slot and keeps its card", () => {
    const probe = { canSee: false, probing: false, query: settled };
    expect(readHealth(probe, words.t, () => fail())).toEqual({
      card: null,
      panel: true,
    });
  });

  it("keeps the card for a failed read, which carries the retry", () => {
    const probe = {
      canSee: true,
      probing: false,
      query: { data: undefined, isError: true, errorUpdateCount: 1 },
    };
    expect(readHealth(probe, words.t, () => fail())).toEqual({
      card: { value: "Could not check", tone: "danger" },
      panel: true,
    });
  });

  it("keeps that card while its retry is in flight", () => {
    const probe = {
      canSee: true,
      probing: false,
      query: { data: undefined, isError: false, errorUpdateCount: 1 },
    };
    expect(readHealth(probe, words.t, () => fail()).panel).toBe(true);
  });

  it("still waits on a first read that has not failed", () => {
    const probe = {
      canSee: true,
      probing: false,
      query: { data: undefined, isError: false, errorUpdateCount: 0 },
    };
    expect(readHealth(probe, words.t, () => fail())).toEqual({
      card: { value: "Checking…" },
      panel: false,
    });
  });

  it("hands a settled report to its own reading", () => {
    const probe = { canSee: true, probing: false, query: settled };
    expect(readHealth(probe, words.t, (data) => ok(data))).toEqual(
      ok("report"),
    );
  });
});

describe("jobsReading", () => {
  it("calls an empty queue idle and draws no card", () => {
    expect(jobsReading(jobs([]), words)).toEqual({
      card: { value: "Idle", tone: "success" },
      panel: false,
    });
  });

  it("keeps the table for a queue that is working, under a clean answer", () => {
    expect(
      jobsReading(jobs([kind({ waiting: 4, running: 1 })]), words),
    ).toEqual({ card: { value: "Keeping up", tone: "success" }, panel: true });
  });

  it("lets only recent dead work and live retries set the tone", () => {
    // A cancelled failure from days ago is history, not a finding.
    expect(jobsReading(jobs([kind({ dead: 3 })], 2), words).card).toEqual({
      value: "Keeping up",
      tone: "success",
    });
    expect(jobsReading(jobs([kind({ retrying: 2 })]), words).card).toEqual({
      value: "2 retrying",
      tone: "warning",
    });
    expect(
      jobsReading(jobs([kind({ retrying: 2, dead_recent: 1 })]), words).card,
    ).toEqual({ value: "1 dead", tone: "danger" });
  });
});

describe("intakeReading", () => {
  it("answers none for a clean window and sums the refused records", () => {
    const report = {
      generated_at: "2026-10-14T09:00:00Z",
      window_days: 7,
      units: [],
    };
    expect(intakeReading(report, words).panel).toBe(false);
    expect(
      intakeReading(
        { ...report, units: [{ unit: "a", refused: 0, refusals: [] }] },
        words,
      ),
    ).toEqual({
      card: { value: "None", tone: "success" },
      panel: false,
    });
    const refused = intakeReading(
      {
        ...report,
        units: [
          { unit: "a", refused: 3, refusals: [] },
          { unit: "b", refused: 1, refusals: [] },
        ],
      },
      words,
    );
    expect(refused).toEqual({
      card: { value: "4 refused", tone: "warning" },
      panel: true,
    });
  });
});

describe("providersReading", () => {
  const href = "#/settings/models";

  it("says not set up, with the way to set one up, when nothing is bound", () => {
    expect(
      providersReading({ providers: [], configured: [] }, words, href),
    ).toEqual({ card: { value: "Not set up", href }, panel: false });
  });

  it("says no bound model can be called when only the fake answers", () => {
    expect(
      providersReading({ providers: [], configured: ["fake"] }, words, href)
        .card,
    ).toEqual({
      value: "Built-in test provider",
      tone: "info",
      detail: "The bound models cannot be called. Add a key under AI models.",
      href,
    });
  });

  it("says only some tiers fall back when real providers are mixed with the fake", () => {
    expect(
      providersReading(
        { providers: [], configured: ["anthropic", "fake"] },
        words,
        href,
      ).card,
    ).toEqual({
      value: "Built-in test provider",
      tone: "info",
      detail:
        "Some tiers answer from the built-in test provider. Add their keys under AI models.",
      href,
    });
  });

  it("answers for real providers that are all answering", () => {
    expect(
      providersReading(
        { providers: [], configured: ["anthropic"] },
        words,
        href,
      ).card,
    ).toEqual({ value: "Answering", tone: "success" });
  });

  it("warns for a degraded provider and alarms for a blocked one", () => {
    const since = "2026-10-14T09:00:00Z";
    const degraded = providersReading(
      {
        providers: [{ provider: "anthropic", health: "degraded", since }],
        configured: ["anthropic"],
      },
      words,
      href,
    );
    expect(degraded.card?.tone).toBe("warning");
    expect(degraded.panel).toBe(true);
    const blocked = providersReading(
      {
        providers: [
          { provider: "anthropic", health: "degraded", since },
          { provider: "openai", health: "unauthorized", since },
        ],
        configured: ["anthropic", "openai"],
      },
      words,
      href,
    );
    expect(blocked.card).toEqual({ value: "2 need attention", tone: "danger" });
  });
});

describe("searchIndexReading", () => {
  const status = {
    status: "idle" as const,
    reindex_needed: false,
    entities_pending: 0,
    configured_identity: "embed-v1",
    populated_identity: "embed-v1",
    updated_at: "2026-10-14T09:00:00Z",
  };

  it("folds an index that is up to date into its slot", () => {
    expect(searchIndexReading(status, words)).toEqual({
      card: { value: "Up to date", tone: "success" },
      panel: false,
    });
  });

  it("keeps the card, and its verbs, while a reindex is due or running", () => {
    const due = searchIndexReading(
      { ...status, reindex_needed: true, entities_pending: 1200 },
      words,
    );
    expect(due.card?.detail).toBe("1,200 records pending");
    expect(due.panel).toBe(true);
    expect(
      searchIndexReading({ ...status, status: "reembedding" }, words).panel,
    ).toBe(true);
  });
});

describe("oldestStamp", () => {
  it("dates the page by its stalest report and skips the unread ones", () => {
    expect(
      oldestStamp([
        "2026-10-14T09:05:00Z",
        undefined,
        "2026-10-14T09:01:00Z",
        "2026-10-14T09:03:00Z",
      ]),
    ).toBe("2026-10-14T09:01:00Z");
    expect(oldestStamp([undefined])).toBeUndefined();
  });
});

function ok(value: string) {
  return { card: { value }, panel: false };
}

function fail(): never {
  throw new Error("a reading that is not settled must not reach its report");
}
