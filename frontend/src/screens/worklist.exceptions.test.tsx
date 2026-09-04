// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { TeamExceptionsPanel } from "./worklist.exceptions";
import { day, renderWorklist } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// What a lead is shown, and the two things the panel must not do.

describe("what needs the lead", () => {
  it("names the basis each row was judged against", async () => {
    stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      exceptions: [
        {
          kind: "response_breached",
          owner: { kind: "user", id: "u1", label: "Lena Fischer" },
          subject: { type: "lead", id: "l1", label: "Kirsten at LOXXESS" },
          since: "2026-09-05T07:00:00Z",
          consequence: "customer_waits",
          // The policy's own state, not a number invented for the reading.
          threshold: "breached",
        },
      ],
    });

    renderPanel();

    expect(await screen.findByText("Kirsten at LOXXESS")).toBeTruthy();
    // The BASIS is on screen. A verdict without it is one a lead cannot
    // dispute, which is the whole reason the server carries the field.
    expect(screen.getByText("breached")).toBeTruthy();
    expect(screen.getByText("Lena Fischer")).toBeTruthy();
  });

  it("says nobody rather than printing a raw id", async () => {
    stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      exceptions: [
        {
          kind: "unassigned",
          // No label and no id: work nobody has taken.
          owner: { kind: "unassigned" },
          subject: { type: "deal", id: "d1", label: "Acme expansion" },
          since: "2026-09-05T07:00:00Z",
          consequence: "deal_drifts",
          threshold: "no owner stated by the lane that raised it",
        },
      ],
    });

    renderPanel();

    await screen.findByText("Acme expansion");
    expect(screen.getByText(en["worklist.exceptions.nobody"])).toBeTruthy();
  });

  it("admits a bounded page is not a clear team", async () => {
    stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      // The server read to its own bound. A lead taking this list for the
      // whole of it would stop looking exactly where the rest begins.
      truncated: true,
      exceptions: [
        {
          kind: "revenue_at_risk",
          owner: { kind: "user", id: "u1", label: "Lena Fischer" },
          subject: { type: "deal", id: "d2", label: "LOXXESS renewal" },
          since: "2026-09-05T07:00:00Z",
          consequence: "deal_drifts",
          threshold: "at or above the pipeline's median open deal",
        },
      ],
    });

    renderPanel();

    await screen.findByText("LOXXESS renewal");
    expect(screen.getByText(en["worklist.exceptions.truncated"])).toBeTruthy();
  });

  it("asks nothing when the reader cannot hold the tier", async () => {
    const fetched = stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      exceptions: [],
    });

    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <TeamExceptionsPanel enabled={false} onOwner={() => {}} />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    // A rep asking earns a 403, and a panel that rendered that as an error
    // would tell them a surface exists which is not theirs.
    expect(fetched.mock.calls.length).toBe(0);
  });
});

function stubExceptions(body: unknown) {
  const fetched = vi.fn(
    async () =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

function renderPanel(onOwner: (id: string) => void = () => {}) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <TeamExceptionsPanel enabled onOwner={onOwner} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The ORDER the two panels sit in, on the page rather than in a component.
//
// The board answers "who is carrying what" and this panel answers "what is
// going wrong". A lead opens the page for the second question, and answering
// the first one first asks them to infer the trouble from three counts per
// teammate — which is the page this one replaces.
//
// Held here because nothing else could hold it: both panels render correctly in
// isolation whichever way round they are drawn, so the claim lives in the
// screen and a future edit that swapped the two blocks would break no test.
describe("the page answers what before who", () => {
  it("draws the exceptions above the team board", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.includes("/worklist/team/exceptions")) {
          return jsonOf({
            as_of: "2026-09-05T09:00:00Z",
            exceptions: [
              {
                kind: "response_breached",
                subject: { type: "person", id: "p-1", label: "Kirsten Bauer" },
                owner: { kind: "user", id: "u-1", label: "Lena Fischer" },
                basis: "past the policy's own deadline",
              },
            ],
            truncated: false,
          });
        }
        if (url.includes("/worklist/team")) {
          return jsonOf({
            as_of: "2026-09-05T09:00:00Z",
            members: [
              {
                user_id: "u-1",
                display_name: "Lena Fischer",
                counts: { waiting: 2, at_risk: 1, overdue: 0 },
              },
            ],
            unassigned: { waiting: 0, at_risk: 0, overdue: 0 },
            truncated: false,
          });
        }
        if (url.includes("/worklist")) {
          return jsonOf(
            day({
              scope_options: ["mine", "team"],
              summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
            }),
          );
        }
        return jsonOf({ data: [] });
      }),
    );
    renderWorklist();

    const exceptions = await screen.findByText(en["worklist.exceptions.title"]);
    const board = await screen.findByText(en["worklist.board.title"]);
    // DOCUMENT_POSITION_FOLLOWING: the board comes after the exceptions in the
    // document. Comparing rendered order rather than asserting both are present
    // — presence passes whichever way round they are drawn, which is exactly
    // the regression this exists to catch.
    expect(
      exceptions.compareDocumentPosition(board) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});

function jsonOf(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

// Who answers for a row, when the reader cannot be told who.
//
// `WorklistOwner` carries a KIND — `user` or `unassigned` — and separately a
// label the caller may not be able to resolve. Those are two different facts
// and the panel was reading only one: any row without a label rendered as
// "Nobody yet".
//
// So an exception a teammate is already carrying was reported to their lead as
// unassigned work. That is not a display nicety. A lead reads "nobody" as "this
// is going nowhere" and takes it on, when somebody is already on it — and the
// row's click sent them to the unassigned scope, which is the one queue the
// work is definitely not in.
describe("an owner the reader cannot name is not nobody", () => {
  const heldByAStranger = {
    as_of: "2026-09-05T09:00:00Z",
    truncated: false,
    exceptions: [
      {
        kind: "response_breached",
        // A real person holds it. The caller may not resolve their name.
        owner: { kind: "user", id: "01a05500-0000-7000-8000-0000000000aa" },
        subject: { type: "lead", id: "l1", label: "Kirsten at LOXXESS" },
        since: "2026-09-01T09:00:00Z",
        consequence: "customer_waits",
        threshold: "first reply past target",
      },
    ],
  };

  it("says the owner is withheld rather than absent", async () => {
    stubExceptions(heldByAStranger);
    renderPanel();

    await screen.findByText(/Kirsten at LOXXESS/);
    expect(
      screen.getByText(en["worklist.exceptions.ownerWithheld"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["worklist.exceptions.nobody"])).toBeNull();
  });

  // The routing reads the KIND, and the case that tells the two readings apart
  // is an `unassigned` owner that still carries an id.
  //
  // Reading `owner.id ?? ""` gets the withheld-name row right by accident — the
  // id is there, so it routes correctly — and gets THIS one wrong: it would
  // send a lead to a person's queue for work the wire says nobody holds. Only a
  // fixture carrying both an `unassigned` kind and an id can fail one reading
  // and pass the other.
  it("opens the unassigned scope for work nobody holds, whatever id rides along", async () => {
    stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      exceptions: [
        {
          kind: "unassigned",
          owner: {
            kind: "unassigned",
            id: "01a05500-0000-7000-8000-0000000000cc",
          },
          subject: { type: "lead", id: "l3", label: "A lead nobody took" },
          since: "2026-09-01T09:00:00Z",
          consequence: "customer_waits",
          threshold: "nobody has taken it",
        },
      ],
    });
    const opened: string[] = [];
    renderPanel((id) => opened.push(id));

    (await screen.findByText(/A lead nobody took/)).click();

    expect(opened).toEqual([""]);
  });

  it("opens that owner's queue when a person holds it", async () => {
    stubExceptions(heldByAStranger);
    const opened: string[] = [];
    renderPanel((id) => opened.push(id));

    (await screen.findByText(/Kirsten at LOXXESS/)).click();

    expect(opened).toEqual(["01a05500-0000-7000-8000-0000000000aa"]);
  });

  it("still says nobody when the wire says nobody", async () => {
    stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      exceptions: [
        {
          kind: "unassigned",
          owner: { kind: "unassigned" },
          subject: { type: "lead", id: "l2", label: "An untaken lead" },
          since: "2026-09-01T09:00:00Z",
          consequence: "customer_waits",
          threshold: "nobody has taken it",
        },
      ],
    });
    renderPanel();

    await screen.findByText(/An untaken lead/);
    expect(screen.getByText(en["worklist.exceptions.nobody"])).toBeTruthy();
  });
});
