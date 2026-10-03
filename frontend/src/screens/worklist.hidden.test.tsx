// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { HiddenBacklogPanel } from "./worklist.hidden";

// The surface that reports the queue's own worst failure.
//
// Its healthy answer is a row of zeros, which is also what a broken read looks
// like — so what this file is about is the cases where the two must not render
// alike: a failed read, and a read the server cut short.

type HiddenBacklog = components["schemas"]["HiddenBacklog"];

function backlog(over: Partial<HiddenBacklog> = {}): HiddenBacklog {
  return {
    as_of: "2026-09-02T09:00:00Z",
    shown: 12,
    set_aside: 0,
    not_sales: 0,
    past_horizon: 0,
    unlinked: 0,
    colleagues: 0,
    informs_us: 0,
    truncated: false,
    clear: true,
    ...over,
  };
}

type HiddenBacklogRows = components["schemas"]["HiddenBacklogRows"];

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "content-type":
        status === 200 ? "application/json" : "application/problem+json",
    },
  });
}

// The panel reads the figures from /worklist/hidden and, once a figure is
// opened, its messages from /worklist/hidden/{rule}.
function draw(
  answer: HiddenBacklog | "fails",
  enabled = true,
  rows: HiddenBacklogRows | "fails" = {
    as_of: "2026-09-02T09:00:00Z",
    rule: "informs_us",
    rows: [],
  },
) {
  const onOpenEmail = vi.fn();
  const rowReads: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string) => {
      const url = typeof input === "string" ? input : input.url;
      const isRows = /\/worklist\/hidden\/[a-z_]+/.test(url);
      if (isRows) {
        rowReads.push(url);
      }
      const body = isRows ? rows : answer;
      return body === "fails" ? json({ code: "unavailable" }, 503) : json(body);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  // The client comes back out for the one case that needs the panel's own read
  // to answer twice: a reading already in the cache, and then a refetch that
  // fails under it.
  return {
    client,
    onOpenEmail,
    rowReads,
    ...render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <HiddenBacklogPanel enabled={enabled} onOpenEmail={onOpenEmail} />
        </LocaleProvider>
      </QueryClientProvider>,
    ),
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the hidden-backlog panel", () => {
  it("names each rule that is holding work back, and how much", async () => {
    draw(backlog({ clear: false, past_horizon: 3, not_sales: 7 }));

    await waitFor(() =>
      expect(screen.getByText("Too old for the Worklist")).toBeTruthy(),
    );
    expect(screen.getByText("3 waiting")).toBeTruthy();
    expect(screen.getByText("Marked not sales work")).toBeTruthy();
    expect(screen.getByText("7 waiting")).toBeTruthy();
  });

  // The model's own hiding rule has its figure like the others, so a wrong
  // verdict about a customer shows up where a lead looks.
  it("counts what the informs_us verdict holds back", async () => {
    draw(backlog({ clear: false, informs_us: 4 }));

    await waitFor(() =>
      expect(screen.getByText("Judged to ask nothing")).toBeTruthy(),
    );
    expect(screen.getByText("4 waiting")).toBeTruthy();
  });

  // A figure opens onto the messages behind it, read only when opened.
  it("lists the messages behind a figure once it is opened", async () => {
    const user = userEvent.setup();
    const { onOpenEmail, rowReads } = draw(
      backlog({ clear: false, informs_us: 1 }),
      true,
      {
        as_of: "2026-09-02T09:00:00Z",
        rule: "informs_us",
        rows: [
          {
            activity_id: "01a0e000-0000-7000-8000-000000000001",
            subject: "Weekly figures",
            since: "2026-09-01T08:00:00Z",
          },
        ],
      },
    );
    await waitFor(() =>
      expect(screen.getByText("Judged to ask nothing")).toBeTruthy(),
    );
    expect(screen.queryByText(/Weekly figures/)).toBeNull();
    expect(rowReads).toEqual([]);

    await user.click(screen.getByText("Judged to ask nothing"));

    expect(await screen.findByText(/Weekly figures/)).toBeTruthy();
    expect(onOpenEmail).not.toHaveBeenCalled();
  });

  it("opens a held-back email in the page's drawer", async () => {
    const user = userEvent.setup();
    const id = "01a0e000-0000-7000-8000-000000000002";
    const { onOpenEmail } = draw(
      backlog({ clear: false, informs_us: 1 }),
      true,
      {
        as_of: "2026-09-02T09:00:00Z",
        rule: "informs_us",
        rows: [
          {
            activity_id: id,
            subject: "Monthly report",
            since: "2026-09-01T08:00:00Z",
            email_summary: {
              activity_id: id,
              occurred_at: "2026-09-01T08:00:00Z",
              version: 1,
              subject: "Monthly report",
              preview: "The figures for August are attached.",
              counterparty: "Dana Buyer",
              direction: "inbound",
              display_status: "workspace",
              move: "none",
              attachment_count: 0,
            },
          },
        ],
      },
    );
    await waitFor(() =>
      expect(screen.getByText("Judged to ask nothing")).toBeTruthy(),
    );
    await user.click(screen.getByText("Judged to ask nothing"));
    await user.click(
      await screen.findByRole("button", { name: /Monthly report/ }),
    );
    expect(onOpenEmail).toHaveBeenCalledWith(id);
  });

  it("says so when a figure's messages cannot be read", async () => {
    const user = userEvent.setup();
    draw(backlog({ clear: false, informs_us: 1 }), true, "fails");
    await waitFor(() =>
      expect(screen.getByText("Judged to ask nothing")).toBeTruthy(),
    );
    await user.click(screen.getByText("Judged to ask nothing"));
    expect(await screen.findByText(/did not load/i)).toBeTruthy();
  });

  it("says so when a rule holds nothing back by the time it is opened", async () => {
    const user = userEvent.setup();
    draw(backlog({ clear: false, informs_us: 1 }));
    await waitFor(() =>
      expect(screen.getByText("Judged to ask nothing")).toBeTruthy(),
    );
    await user.click(screen.getByText("Judged to ask nothing"));
    expect(
      await screen.findByText("Nothing is held back by this rule now."),
    ).toBeTruthy();
  });

  // A rule holding nothing back is not news. Four rows of zeros would bury the
  // one figure that found something.
  it("draws only the rules that found something", async () => {
    draw(backlog({ clear: false, past_horizon: 3 }));

    await waitFor(() =>
      expect(screen.getByText("Too old for the Worklist")).toBeTruthy(),
    );
    expect(screen.queryByText("Set aside by you")).toBeNull();
    expect(screen.queryByText("Marked not sales work")).toBeNull();
  });

  // THE failure this whole reading exists for. A read cut short by its own scan
  // bound reports every difference as zero, so the numbers look perfect at the
  // moment the check stopped working. The caveat has to be on screen before any
  // figure a reader might otherwise trust.
  it("says the figures are floors when the read was cut short", async () => {
    draw(backlog({ clear: false, truncated: true, past_horizon: 2 }));

    await waitFor(() =>
      expect(screen.getByText(/these figures are minimums/)).toBeTruthy(),
    );
  });

  // At the limit every figure is the difference of two capped reads, so a
  // zero there means "not counted". The rule stays on screen and openable
  // instead of vanishing behind a caveat over an empty list.
  it("keeps a rule it could not count on screen at the reading limit", async () => {
    draw(backlog({ clear: false, truncated: true, past_horizon: 0 }));

    await waitFor(() =>
      expect(screen.getByText("Too old for the Worklist")).toBeTruthy(),
    );
    expect(screen.getAllByText("Not counted").length).toBeGreaterThan(0);
  });

  // Zeros are this surface's HEALTHY answer, so a failed read drawn as zeros
  // would report perfect health exactly when the guardrail broke.
  it("says it could not read rather than drawing a clean bill of health", async () => {
    draw("fails");

    // The PRESENT text, not merely the absent one: asserting only that the
    // clear message is missing passes against a component that rendered
    // nothing at all, which is how the first version of this test survived a
    // mutation that dropped the error state entirely.
    await waitFor(() =>
      expect(screen.getByText(/Some data did not load/)).toBeTruthy(),
    );
    expect(screen.queryByText(/Nothing is hidden/)).toBeNull();
  });

  // The same failure from the other side: the READING is gone from the body on
  // a failed read, but the cached payload outlives it, so the figure in the
  // footer band went on reporting what the queue carried beside a body saying
  // the guardrail could not be read. A number standing next to an unreadable
  // result is taken for the answer.
  it("withholds the queue's own figure when a refetch fails under it", async () => {
    const { client } = draw(backlog({ clear: false, past_horizon: 3 }));
    await waitFor(() =>
      expect(screen.getByText("The Worklist shows 12.")).toBeTruthy(),
    );

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("", { status: 503 })),
    );
    await act(async () => {
      await client.refetchQueries();
    });

    await waitFor(() =>
      expect(screen.getByText(/Some data did not load/)).toBeTruthy(),
    );
    expect(screen.queryByText(/The Worklist shows/)).toBeNull();
  });

  it("says so plainly when nothing is held back", async () => {
    draw(backlog());

    await waitFor(() =>
      expect(screen.getByText(/Nothing is hidden/)).toBeTruthy(),
    );
  });

  // A seat with no route to this reading does not fire the request. The figures
  // are gated server-side either way, so this is about not making a call behind
  // a reader's back rather than about safety.
  it("asks nothing when the reader has no tier for it", () => {
    const fetched = vi.fn();
    vi.stubGlobal("fetch", fetched);
    draw(backlog(), false);

    expect(fetched).not.toHaveBeenCalled();
  });
});
