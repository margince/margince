// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import {
  DispositionVerbs,
  PutDownByThumb,
  SWIPE_SIDES,
} from "./worklist.dispositions";
import type { WorklistDisposition, WorklistItem } from "./worklist.queries";

// How a row is put down when there is no width for the verbs.
//
// Why the verbs cannot stay is the width arithmetic in
// design-system/swiperow.tsx's header. These assert the SWAP and that no
// judgement is lost by it; the gesture arithmetic is
// design-system/swiperow.test.tsx's.

// EVERY judgement the map places, not a fixture's favourite two. A census that
// names its own subjects proves only that they were named: the reachability
// test below walks this list, so a third judgement cannot be silently
// unreachable while the suite stays green.
const EVERY_JUDGEMENT = Object.keys(SWIPE_SIDES) as WorklistDisposition[];

function row(dispositions: readonly WorklistDisposition[]): WorklistItem {
  return {
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: "Anna Weber is waiting",
    because: [],
    actions: [],
    dispositions: [...dispositions],
  };
}

// The viewport the hook reads. jsdom's matchMedia always answers false and
// cannot change its mind, so the width under test is stated here.
function atWidth(folded: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query.includes("720px") ? folded : false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

function draw(dispositions: readonly WorklistDisposition[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const item = row(dispositions);
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {/* The row as worklist.row.tsx mounts it: the gesture wraps the work
              and the verbs are drawn inside it, which is what puts the swipe on
              the row rather than on one control in it. */}
          <PutDownByThumb item={item}>
            <p>Anna Weber is waiting</p>
            <DispositionVerbs item={item} />
          </PutDownByThumb>
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

const verb = (disposition: WorklistDisposition) =>
  en[`worklist.disposition.verb.${disposition}`];

// One flick, from `from` to `to`. Past the 56px threshold and horizontal, so
// swipeSide reads it as an answer.
function swipe(from: number, to: number) {
  const target = screen.getByTestId("swipe-row");
  fireEvent.pointerDown(target, { clientX: from, clientY: 0, isPrimary: true });
  fireEvent.pointerUp(target, { clientX: to, clientY: 0, isPrimary: true });
}

const FORWARD: [number, number] = [0, 90];
const BACK: [number, number] = [90, 0];

describe("putting a row down below the fold", () => {
  it("draws the verbs as buttons where there is width for them", () => {
    atWidth(false);
    draw(EVERY_JUDGEMENT);

    for (const disposition of EVERY_JUDGEMENT) {
      expect(
        screen.getByRole("button", { name: verb(disposition) }),
      ).toBeInTheDocument();
    }
    expect(screen.queryByTestId("swipe-row")).toBeNull();
  });

  // The band is what pushed the row over its ceiling, so below the fold the
  // verbs must be GONE rather than rearranged: a smaller row of the same
  // buttons is the defect this change exists to remove.
  it("draws no verb buttons below the fold", () => {
    atWidth(true);
    draw(EVERY_JUDGEMENT);

    for (const disposition of EVERY_JUDGEMENT) {
      expect(
        screen.queryByRole("button", { name: verb(disposition) }),
      ).toBeNull();
    }
  });

  // THE CENSUS. Every judgement the server offered has to be reachable by some
  // number of flicks in some direction, because a verb that exists only on a
  // wide screen is a capability a phone lost — and losing one quietly is what
  // the row ceiling was already doing.
  it("leaves no offered judgement unreachable below the fold", () => {
    atWidth(true);
    draw(EVERY_JUDGEMENT);

    const reached = new Set<string>();
    // Twice round each side, so a direction carrying two judgements is walked
    // rather than sampled once.
    for (const [from, to] of [FORWARD, BACK, FORWARD, BACK]) {
      swipe(from, to);
      for (const disposition of EVERY_JUDGEMENT) {
        if (
          screen.queryByRole("button", { name: verb(disposition) }) !== null
        ) {
          reached.add(disposition);
        }
      }
      const keep = screen.queryByRole("button", {
        name: en["worklist.disposition.swipeCancel"],
      });
      if (keep !== null) {
        fireEvent.click(keep);
      }
    }

    for (const disposition of EVERY_JUDGEMENT) {
      expect(
        reached.has(disposition),
        `${disposition} cannot be reached by any swipe, so a phone lost the verb`,
      ).toBe(true);
    }
  });

  // A row offering only the judgements one direction carries must still answer
  // that direction. An earlier version keyed the whole surface on the presence
  // of a snooze and drew an empty box without one.
  it("answers a row whose only judgement is on one side", () => {
    atWidth(true);
    draw(["not_mine"]);
    swipe(...BACK);

    expect(
      screen.getByRole("button", { name: verb("not_mine") }),
    ).toBeInTheDocument();
  });

  it("draws no gesture at all on a row the server offers nothing for", () => {
    atWidth(true);
    draw([]);

    expect(screen.queryByTestId("swipe-row")).toBeNull();
  });
});
