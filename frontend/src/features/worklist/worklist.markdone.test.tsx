// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { rowMenuName } from "../../testing/worklist.testkit";
import { swipeActions } from "./worklist.dispositions";
import type { WorklistDisposition, WorklistItem } from "./worklist.queries";
import { RowActs } from "./worklist.rowverbs";
import { knownDispositions } from "./worklist.verbbutton";

// Done and Not mine on a waiting row: what each says it does, before and after
// the press. The server decides which of them a row offers; these draw the
// row a colleague's message gets, which carries both.

const TITLE = "Anna Weber is waiting";

function colleaguesRow(): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "01a05500-0000-7000-8000-0000000000c1",
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: TITLE,
    because: [],
    actions: [],
    dispositions: ["handled", "snooze", "not_mine", "not_sales"],
  };
}

async function sentBody(
  fetch: ReturnType<typeof vi.fn>,
  call = 0,
): Promise<Record<string, unknown>> {
  const [input] = fetch.mock.calls[call] ?? [];
  expect(input, "nothing was sent").toBeInstanceOf(Request);
  return JSON.parse(await (input as Request).clone().text()) as Record<
    string,
    unknown
  >;
}

function sentTo(fetch: ReturnType<typeof vi.fn>, call = 0): string {
  const [input] = fetch.mock.calls[call] ?? [];
  return `${(input as Request).method} ${new URL((input as Request).url).pathname}`;
}

function draw(item: WorklistItem = colleaguesRow()) {
  const fetch = vi.fn(
    async () =>
      new Response("{}", {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <RowActs item={item} title={TITLE} href={undefined} owner="" />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return fetch;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const MENU = rowMenuName(TITLE);

describe("Done on a waiting row", () => {
  // Done finishes the row, so it stays on the line ahead of the row's menu.
  // The judgements that only move the row off the list are lines of it.
  it("stands before the row's menu and sends handled", async () => {
    const user = userEvent.setup();
    const fetch = draw();

    const done = screen.getByRole("button", {
      name: en["worklist.disposition.verb.handled"],
    });
    expect(
      screen
        .getAllByRole("button")
        .map(
          (button) => button.textContent || button.getAttribute("aria-label"),
        ),
    ).toEqual([en["worklist.disposition.verb.handled"], MENU]);

    await user.click(done);

    expect(sentTo(fetch)).toBe("PUT /v1/worklist/done");
    expect(await sentBody(fetch)).toEqual({
      rows: [{ source: "customer_waiting", row_id: colleaguesRow().id }],
    });
    expect(
      await screen.findByText(en["worklist.disposition.done.handled"]),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: en["worklist.disposition.undo"] }),
    ).toBeTruthy();
  });

  // Every other row takes the same Done, says it comes back when something new
  // happens, and its Undo takes it back through the same endpoint.
  it("takes a deal at risk off the list and undoes it", async () => {
    const user = userEvent.setup();
    const deal: WorklistItem = {
      ...colleaguesRow(),
      id: "01a05500-0000-7000-8000-0000000000d1",
      source: "deal_at_risk",
      category: "deals_at_risk",
      dispositions: ["handled"],
    };
    const fetch = draw(deal);

    await user.click(
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.handled"],
      }),
    );
    expect(sentTo(fetch)).toBe("PUT /v1/worklist/done");
    expect(
      await screen.findByText(en["worklist.disposition.done.handledRow"]),
    ).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: en["worklist.disposition.undo"] }),
    );
    expect(sentTo(fetch, 1)).toBe("POST /v1/worklist/done/undo");
    expect(await sentBody(fetch, 1)).toEqual({
      rows: [{ source: "deal_at_risk", row_id: deal.id }],
    });
  });

  it("leads the forward swipe", () => {
    const put = vi.fn();
    const offered: WorklistDisposition[] = ["handled", "snooze", "not_mine"];
    const t = ((key: string) => key) as Parameters<typeof swipeActions>[1];

    swipeActions(offered, t, put, { start: 0, end: 0 }).end?.onAct();

    expect(put).toHaveBeenCalledWith("handled");
  });
});

describe("Not mine on a colleague's row", () => {
  it("says it comes back when the record changes hands", async () => {
    const user = userEvent.setup();
    draw();

    await user.click(screen.getByRole("button", { name: MENU }));
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.not_mine"],
      }),
    );

    const note = await screen.findByText(
      en["worklist.disposition.done.not_mine"],
    );
    expect(note.textContent).not.toMatch(/owner still sees/i);
  });

  it("explains itself to a reader who reaches it by keyboard", async () => {
    const user = userEvent.setup();
    draw();
    await user.tab();
    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: MENU }),
    );
    await user.keyboard("{Enter}");
    const notMine = await screen.findByRole("button", {
      name: en["worklist.disposition.verb.not_mine"],
    });

    for (let step = 0; document.activeElement !== notMine; step++) {
      expect(step, "tabbing never reached Not mine").toBeLessThan(10);
      await user.tab();
    }

    const tip = await screen.findByRole("tooltip");
    expect(tip.textContent).toBe(en["worklist.disposition.hint.not_mine"]);
    expect(notMine.getAttribute("aria-describedby")).toContain(tip.id);
  });
});

// A newer server may offer a verb this build has no label for. It is left out
// rather than drawn as a raw catalog key.
describe("a disposition this build does not know", () => {
  it("is dropped from what the row offers", () => {
    expect(
      knownDispositions(["handled", "archive_forever", "not_mine"]),
    ).toEqual(["handled", "not_mine"]);
  });
});
