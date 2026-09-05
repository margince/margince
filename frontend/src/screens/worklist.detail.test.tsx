// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";

// The supporting line, and the words for a group's cause.
//
// Twelve sources send a sentence saying the decisive thing about their row —
// which mailbox stopped, why a message bounced, why a send was held, which rule
// failed and how — and the row drew it for exactly one of them. A rep looking at
// "Automation failed" learned nothing about which rule or why, and the answer
// was already on the wire.
//
// The reason it was gated is real and now fixed on the other side: three sources
// used the field as a typed channel — two wrote a bare day count, one wrote the
// marker words the queue groups by — so drawing it would have printed "90" under
// one title and "machine_sender" under another. Those three send their values
// typed now.
//
// The cases below are a SAMPLE of the sources that send prose, not a census: one
// per shape of sentence, plus the one source still held back. What holds the
// whole set is the server-side gate, which walks the renderers themselves
// (backend/internal/compose/attention/causeref_test.go).

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function day(queue: WorklistItem[]): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    summary: {
      urgent: 0,
      due: queue.length,
      lower_priority: 0,
      total: queue.length,
    },
    sources_unavailable: [],
    readings: {
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
    reach: [],
    counts: [],
  };
}

function draw(queue: WorklistItem[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(day(queue))),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// One case per source that sends prose, each asserting the DECISIVE line — the
// half a rep needs to know what happened, which is the half they could not see.
const sourcesWithProse: ReadonlyArray<{
  name: string;
  row: WorklistItem;
  says: string;
}> = [
  {
    name: "a mailbox that stopped capturing",
    says: "lena.fischer@margince.test",
    row: {
      id: "c1",
      source: "capture_health",
      category: "system",
      level: 6,
      consequence: "data_drifts",
      kind: "disconnected",
      title: "A mailbox stopped syncing",
      detail: "lena.fischer@margince.test",
      because: [],
      actions: [],
    },
  },
  {
    name: "a message that bounced",
    says: "The address does not exist at that domain.",
    row: {
      id: "b1",
      source: "bounce",
      category: "customer_waiting",
      level: 3,
      consequence: "buyer_waits",
      title: "Your quote never arrived",
      detail: "The address does not exist at that domain.",
      because: [],
      actions: [],
    },
  },
  {
    name: "a send that was held",
    says: "Held: the recipient has not consented to marketing mail.",
    row: {
      id: "u1",
      source: "undelivered",
      category: "customer_waiting",
      level: 3,
      consequence: "buyer_waits",
      title: "A message never left",
      detail: "Held: the recipient has not consented to marketing mail.",
      because: [],
      actions: [],
    },
  },
  {
    name: "an automation that failed",
    says: "The webhook target answered 500 five times.",
    row: {
      id: "a1",
      source: "automation_run",
      category: "system",
      level: 6,
      consequence: "data_drifts",
      title: "Notify sales on a new lead",
      detail: "The webhook target answered 500 five times.",
      because: [],
      actions: [],
    },
  },
  {
    name: "an AI task that broke",
    says: "Acme Renewal",
    row: {
      id: "w1",
      source: "ai_work_health",
      category: "system",
      level: 6,
      consequence: "data_drifts",
      title: "A recap did not generate",
      detail: "Acme Renewal",
      because: [],
      actions: [],
    },
  },
  {
    name: "a colleague waiting on an introduction",
    says: "They asked to be introduced to the buyer at Turbinenbau.",
    row: {
      id: "i1",
      source: "introduction_request",
      category: "decisions",
      level: 5,
      consequence: "work_blocked",
      title: "Katrin asked for an introduction",
      detail: "They asked to be introduced to the buyer at Turbinenbau.",
      because: [],
      actions: [],
    },
  },
];

describe("the supporting line each source sends", () => {
  it.each(sourcesWithProse)(
    "says the decisive thing about $name",
    async ({ row, says }) => {
      draw([row]);
      expect(await screen.findByText(says)).toBeTruthy();
    },
  );

  // The source that sends its facts in its own vocabulary, drawn now that the
  // client writes the sentence. This is the case that keeps "render every
  // source" honest: the rule is prose, not "whatever a source happens to send".
  it.each([
    {
      what: "a degraded read budget",
      kind: "budget_degraded",
      detail: "shed",
      says: "Over the read budget: reads are being served from the copy rather than live.",
      never: "shed",
    },
    {
      what: "a failing sweep",
      kind: "sync_failing",
      detail: "rate_limited",
      says: "Not syncing — the other system is limiting how often we may ask.",
      never: "rate_limited",
    },
    {
      what: "stale object classes",
      kind: "objects_stale",
      detail: "deals, contacts",
      says: "Out of date here: deals, people.",
      never: "deals, contacts",
    },
  ])(
    "says $what in the reader's own words",
    async ({ kind, detail, says, never }) => {
      draw([
        {
          id: "s1",
          source: "sync_health",
          category: "system",
          level: 6,
          consequence: "data_drifts",
          kind,
          detail,
          because: [],
          actions: [],
        },
      ]);

      expect(await screen.findByText(says)).toBeTruthy();
      // The producer's word never reaches the page. `deals, contacts` reads
      // almost like the sentence, which is exactly why it is asserted: a
      // renderer that fell back to the raw field would look right at a glance.
      expect(screen.queryByText(never)).toBeNull();
    },
  );

  // A value this build does not know draws NO line rather than its own key —
  // which is what every sync row did before the sentence existed, so an
  // unrecognised value is never worse than the old behaviour.
  it("draws no line for a condition value it cannot put into words", async () => {
    draw([
      {
        id: "s2",
        source: "sync_health",
        category: "system",
        level: 6,
        consequence: "data_drifts",
        kind: "budget_degraded",
        detail: "a_band_this_build_never_shipped",
        because: [],
        actions: [],
      },
    ]);

    expect(
      await screen.findByText("The CRM sync needs attention"),
    ).toBeTruthy();
    expect(screen.queryByText("a_band_this_build_never_shipped")).toBeNull();
  });
});

describe("a group names what is broken", () => {
  // THE defect: `cause` is the identity the group was formed on, and reads like
  // one. Interpolated into the sentence it put "automation_run:<uuid> failed 12
  // times" in front of a rep — a string they cannot act on and cannot tell from
  // a bug.
  it("draws the label and never the identity behind it", async () => {
    draw([
      {
        id: "g1",
        source: "automation_run",
        category: "system",
        level: 6,
        consequence: "data_drifts",
        because: [],
        actions: [],
        batch: {
          key: "system_incident",
          count: 12,
          cause: "automation_run:01a05500-0000-7000-8000-0000000000a1",
          label: "Notify sales on a new lead",
        },
      },
    ]);

    expect(
      await screen.findByText(/Notify sales on a new lead failed 12 times/),
    ).toBeTruthy();
    // Asserted over the WHOLE rendered page rather than the one line, because
    // an identity is a defect wherever it surfaces — and a uuid is the shape
    // that gives it away.
    expect(document.body.textContent ?? "").not.toMatch(
      /[0-9a-f]{8}-[0-9a-f]{4}-/,
    );
    expect(document.body.textContent ?? "").not.toContain("automation_run:");
  });

  // A lane that minted no name for the condition. The row says what KIND of
  // thing broke and stops — it must not fall back to the identity, which is the
  // one string worse than a vague sentence.
  it("falls back to a generic phrase, never to the identity", async () => {
    draw([
      {
        id: "g2",
        source: "automation_run",
        category: "system",
        level: 6,
        consequence: "data_drifts",
        because: [],
        actions: [],
        batch: {
          key: "system_incident",
          count: 8,
          cause: "automation_run:01a05500-0000-7000-8000-0000000000b2",
        },
      },
    ]);

    expect(await screen.findByText(/failed 8 times/)).toBeTruthy();
    expect(document.body.textContent ?? "").not.toContain("automation_run:");
    expect(document.body.textContent ?? "").not.toMatch(
      /[0-9a-f]{8}-[0-9a-f]{4}-/,
    );
  });
});
