// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { UTC_ZONE } from "../../../format/timezone";
import {
  type Locale,
  LocaleProvider,
  translate,
  translatePlural,
} from "../../../i18n";
import { en } from "../../../i18n/en";
import { deal, stages } from "../deals.testkit";
import { DealFigures, timelineEvents } from "./dealfigures";
import type { DealStandingInputs } from "./dealstanding";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Activity = components["schemas"]["Activity"];

const AS_OF = "2026-05-30T09:00:00Z";
const ENGLISH: Locale = "en";

function show(over: Partial<DealStandingInputs>) {
  const inputs: DealStandingInputs = {
    deal: deal({ stage_id: "s2" }),
    stages,
    asOf: AS_OF,
    activities: [],
    offers: [],
    coverage: { withheld: false, pending: false },
    ladder: { advancing: false, advanceRefused: false, onAdvance: () => {} },
    ...over,
  };
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <DealFigures inputs={inputs} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function stageCell(): HTMLElement {
  const label = screen.getByText(en["deal.standing.figure.daysInStage"]);
  const cell = label.closest("section");
  if (!(cell instanceof HTMLElement)) {
    throw new Error("the Days in stage figure has no cell");
  }
  return cell;
}

function stubHistory(entries: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({ data: entries, page: { has_more: false } }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
    ),
  );
}

function arrival(stageId: string, at: string) {
  return {
    id: `h-${stageId}`,
    entity_type: "deal",
    entity_id: "d1",
    field: "stage_id",
    new_value: stageId,
    changed_at: at,
    actor_type: "human",
    actor_id: "u1",
  };
}

describe("Days in stage", () => {
  it("counts the days since the deal arrived in its stage, and names the stage", async () => {
    stubHistory([arrival("s2", "2026-05-20T09:00:00Z")]);
    show({});
    expect(await within(stageCell()).findByText("10 days")).toBeInTheDocument();
    expect(
      within(stageCell()).getByText(
        stages.find((stage) => stage.id === "s2")?.name ?? "",
      ),
    ).toBeInTheDocument();
  });

  it("says not shown, never zero, when the history names no arrival", async () => {
    stubHistory([]);
    show({});
    expect(
      await within(stageCell()).findByText(en["record.notShown"]),
    ).toBeInTheDocument();
    expect(within(stageCell()).queryByText(/day/)).toBeNull();
  });
});

it("reads the probability off the deal's stage, and says so", () => {
  show({});
  expect(screen.getByText("40%")).toBeInTheDocument();
  expect(
    screen.getByText(en["deal.standing.figure.stageDefault"]),
  ).toBeInTheDocument();
});

describe("what the axis marks", () => {
  const words = {
    t: (
      key: Parameters<typeof translate>[1],
      params?: Record<string, string>,
    ) => translate("en", key, params),
    plural: (
      base: Parameters<typeof translatePlural>[1],
      count: number,
      params?: Record<string, string>,
    ) => translatePlural("en", base, count, params),
    locale: ENGLISH,
    zone: UTC_ZONE,
  };
  const row = (over: Partial<Activity>): Activity => ({
    occurred_precision: "instant",
    offers_email: false,
    id: "row",
    kind: "meeting",
    occurred_at: AS_OF,
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: AS_OF,
    updated_at: AS_OF,
    ...over,
  });
  const marks = (over: Partial<DealStandingInputs>) =>
    timelineEvents(
      {
        deal: deal({ expected_close_date: "2026-06-30" }),
        stages,
        asOf: AS_OF,
        activities: [],
        offers: [],
        coverage: { withheld: false, pending: false },
        ladder: {
          advancing: false,
          advanceRefused: false,
          onAdvance: () => {},
        },
        ...over,
      },
      words,
    ).map((event) => [event.key, event.kind]);

  it("marks what happened, the buyer's last word, what is ahead and the close", () => {
    expect(
      marks({
        activities: [
          row({
            id: "held",
            meeting_status: "held",
            occurred_at: "2026-05-02T09:00:00Z",
          }),
          row({
            id: "gone",
            meeting_status: "canceled",
            occurred_at: "2026-05-03T09:00:00Z",
          }),
          row({
            id: "mail",
            kind: "email",
            direction: "inbound",
            voice: {
              author: {
                kind: "contact_record",
                display_name: "Buyer",
                is_reader: false,
              },
            },
            occurred_at: "2026-05-20T09:00:00Z",
          }),
          row({
            id: "booked",
            meeting_status: "booked",
            occurred_at: "2026-06-10T09:00:00Z",
          }),
          row({ id: "late", kind: "task", due_at: "2026-05-25T00:00:00Z" }),
          row({ id: "due", kind: "task", due_at: "2026-06-05T00:00:00Z" }),
        ],
      }),
    ).toEqual([
      ["held", "done"],
      ["last-mail", "last"],
      ["booked", "ahead"],
      ["late", "late"],
      ["due", "ahead"],
      ["close", "end"],
    ]);
  });

  it("keeps the five newest conversations held", () => {
    const held = Array.from({ length: 7 }, (_, index) =>
      row({
        id: `m${index}`,
        kind: "call",
        occurred_at: `2026-05-0${index + 1}T09:00:00Z`,
      }),
    );
    const kept = marks({ activities: held }).filter(
      ([, kind]) => kind === "done",
    );
    expect(kept.map(([key]) => key)).toEqual(["m6", "m5", "m4", "m3", "m2"]);
  });

  it("draws no close on a deal that already closed", () => {
    expect(
      marks({
        deal: deal({ status: "won", expected_close_date: "2026-05-01" }),
      }),
    ).toEqual([]);
  });
});
