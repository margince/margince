// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import {
  DispositionVerbs,
  PutDownByThumb,
  SNOOZE_DAYS,
  SNOOZE_SPANS,
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

// The row as worklist.row.tsx mounts it, without the fetch stub — for the tests
// that need to control the response themselves.
function rowUnderTest(dispositions: readonly WorklistDisposition[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const item = row(dispositions);
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <PutDownByThumb item={item}>
            <p>Anna Weber is waiting</p>
            <DispositionVerbs item={item} />
          </PutDownByThumb>
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>
  );
}

function draw(dispositions: readonly WorklistDisposition[]) {
  const fetchSpy = vi.fn(
    async () =>
      new Response(JSON.stringify({}), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetchSpy);
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
  return fetchSpy;
}

// Two rows of the same shape, for the assertions that only fail on a page with
// more than one menu in it.
function drawRows(count: number) {
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
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {Array.from({ length: count }, (_, at) => {
            const item = { ...row(EVERY_JUDGEMENT), id: `row-${at}` };
            return (
              <PutDownByThumb key={item.id} item={item}>
                <p>{item.title}</p>
                <DispositionVerbs item={item} />
              </PutDownByThumb>
            );
          })}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The body of the one write, read off the Request the generated client sends.
// An `init.body` lookup finds null on every call and would report "nothing was
// sent" for a request that went out correctly.
async function sentBody(
  fetchSpy: ReturnType<typeof vi.fn>,
): Promise<Record<string, unknown>> {
  const [input] = fetchSpy.mock.calls[0] ?? [];
  const request = input instanceof Request ? input.clone() : undefined;
  const raw = request ? await request.text() : "";
  return raw === "" ? {} : (JSON.parse(raw) as Record<string, unknown>);
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

  // THE KEYBOARD PATH. A swipe is not a control a keyboard, a switch or a
  // screen reader can operate, so a fold that left the gesture as the only
  // route would take the three judgements away from every reader who does not
  // point. The menu is one tab stop carrying all three — not the 44px band
  // back, which is the height the fold removed.
  it("offers every judgement to a keyboard below the fold", async () => {
    const user = userEvent.setup();
    atWidth(true);
    // TWO rows, because one hides the defect: with a single menu the portalled
    // panel happens to be next in document order, and a test that tabs once
    // passes over a page where every other row's trigger comes first.
    drawRows(2);

    // Reachable by TAB, not merely present: a control a keyboard cannot land
    // on is the gap this closes, not a fix for it.
    await user.tab();
    const trigger = screen.getAllByRole("button", {
      name: en["worklist.disposition.menu"],
    })[0];
    expect(document.activeElement).toBe(trigger);

    await user.keyboard("{Enter}");

    // FOCUS IS INSIDE THE PANEL, which is the whole claim. The panel is
    // portalled to the body, so asserting the items merely RENDER proves
    // nothing about reaching them: a reader would Tab to the next row's
    // trigger, then through the rest of the page, and meet these last.
    const first = screen.getByRole("button", {
      name: verb(EVERY_JUDGEMENT[0]),
    });
    expect(document.activeElement).toBe(first);

    // And Tab walks the rest of them rather than leaving for the page.
    for (const disposition of EVERY_JUDGEMENT.slice(1)) {
      await user.tab();
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: verb(disposition) }),
      );
    }
  });

  // BOTH PLACEMENTS SHARE THE ROW'S ONE WRITE.
  //
  // The menu and the swipe are two placements of one capability. Held apart
  // they are two mutations with two `pending` flags and neither disables the
  // other, so a row being written from one placement looks idle to the other.
  //
  // The flag is what the two share, so the flag is what this reads: a write
  // started from the GESTURE has to reach the MENU'S item. A second mutation
  // leaves each placement reading its own, and the menu offers a fresh press
  // over a row already going to the server.
  it("shows the menu a write the gesture started", async () => {
    const user = userEvent.setup();
    atWidth(true);
    // A write that does not settle, so `pending` is observable rather than a
    // state the assertion races.
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    render(rowUnderTest(EVERY_JUDGEMENT));

    // Start a write from the gesture and confirm it.
    swipe(...FORWARD);
    await user.click(
      screen.getAllByRole("button", { name: verb("snooze") })[0],
    );

    // The menu's own item now reads that write as in flight.
    await user.click(
      screen.getByRole("button", { name: en["worklist.disposition.menu"] }),
    );
    await waitFor(() => {
      const items = screen.getAllByRole("button", { name: verb("snooze") });
      expect(
        items.some((one) => one.getAttribute("aria-disabled") === "true"),
        "the menu offers a fresh press over a row already being written, so " +
          "the two placements are not sharing one mutation",
      ).toBe(true);
    });
  });

  // EVERY SPAN SURVIVES THE FOLD, and reaches the wire as itself.
  //
  // The gesture sends the default day and nothing else, so a fold that carried
  // only the verbs left a rep who knows a customer is away all week pressing
  // the same button every morning — the state the spans were added to end.
  // They are menu lines below the fold because a line costs no height where a
  // fourth 44px control does.
  it("offers every snooze span below the fold, and sends the one pressed", async () => {
    const user = userEvent.setup();
    atWidth(true);
    const fetchSpy = draw(EVERY_JUDGEMENT);

    await user.click(
      screen.getByRole("button", { name: en["worklist.disposition.menu"] }),
    );

    // The whole span vocabulary, derived rather than named here: a census that
    // lists its own subjects proves only that they were listed.
    for (const days of SNOOZE_SPANS) {
      const line =
        days === SNOOZE_DAYS
          ? en["worklist.disposition.verb.snooze"]
          : en["worklist.disposition.snoozeForDays_other"].replace(
              "{value}",
              String(days),
            );
      expect(
        screen.getByRole("button", { name: line }),
        `a reader below the fold cannot snooze for ${days} day(s)`,
      ).toBeInTheDocument();
    }

    // And the span REACHES THE SERVER. A line that sent the default day would
    // read as three choices offering one answer.
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.disposition.snoozeForDays_other"].replace(
          "{value}",
          "7",
        ),
      }),
    );
    await waitFor(() => expect(fetchSpy.mock.calls.length).toBe(1));
    const sent = await sentBody(fetchSpy);
    const until = new Date(String(sent.snoozed_until));
    const days = Math.round(
      (until.getTime() - Date.now()) / (24 * 60 * 60 * 1000),
    );
    expect(days, "the seven-day line sent a different span").toBe(7);
  });

  it("draws no gesture at all on a row the server offers nothing for", () => {
    atWidth(true);
    draw([]);

    expect(screen.queryByTestId("swipe-row")).toBeNull();
  });
});
