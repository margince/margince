/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { BriefFeed } from "./brief.feed";
import { readingsDay, taskRow, waitingEmailRow } from "./brief.fixtures";
import { render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/** The row in hand, drawn whole on the panel's left. */
function inHand(container: HTMLElement) {
  const lead = container.querySelector(".brief-triage-lead");
  if (!(lead instanceof HTMLElement)) throw new Error("no row is in hand");
  return within(lead);
}

it("renders the server focus even when the queue page contains different rows", () => {
  stubApi({});
  const rows = Array.from({ length: 9 }, (_, index) =>
    taskRow(`task-${index}`, `Call buyer ${index}`),
  );
  const { container } = render(
    <BriefFeed
      day={{
        ...readingsDay({}, [rows[8]]),
        focus: { items: rows.slice(0, 6), total: 9, urgent_remaining: 0 },
      }}
      state="ready"
    />,
  );
  // The day's first row is in hand, drawn whole; the queue beside it names
  // every focus row in the server's order, with its rank in the reader's own
  // numerals.
  expect(
    [...container.querySelectorAll(".worklist-row-title")].map(
      (row) => row.textContent,
    ),
  ).toEqual([rows[0].title]);
  expect(
    container.querySelector(".brief-focus-item-inhand .brief-focus-rank")
      ?.textContent,
  ).toBe("1");
  expect(
    [...container.querySelectorAll(".brief-focus-item-title")].map(
      (item) => item.textContent,
    ),
  ).toEqual(rows.slice(0, 6).map((row) => row.title));
  expect(
    [...container.querySelectorAll(".brief-focus-list > li")].map(
      (item) => item.querySelector(".brief-focus-rank")?.textContent,
    ),
  ).toEqual(["1", "2", "3", "4", "5", "6"]);
  expect(screen.getByText("1 of 6")).toBeTruthy();
});

it("puts a queued row in hand when it is pressed, and says where it stands", async () => {
  stubApi({});
  const user = userEvent.setup();
  const rows = Array.from({ length: 3 }, (_, index) =>
    taskRow(`task-${index}`, `Call buyer ${index}`),
  );
  const { container } = render(
    <BriefFeed day={readingsDay({}, rows)} state="ready" />,
  );
  await user.click(screen.getByRole("button", { name: /Call buyer 2/ }));
  expect(
    [...container.querySelectorAll(".worklist-row-title")].map(
      (row) => row.textContent,
    ),
  ).toEqual([rows[2].title]);
  expect(
    container.querySelector(".brief-focus-item-inhand .brief-focus-rank")
      ?.textContent,
  ).toBe("3");
  expect(screen.getByText("3 of 3")).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: /Call buyer 2/ })
      .getAttribute("aria-pressed"),
  ).toBe("true");
});

it("opens the full queue instead of growing focus when more pages exist", () => {
  stubApi({});
  render(
    <BriefFeed
      day={{
        ...readingsDay({}, [taskRow("t", "Call the buyer")]),
        next_cursor: "next",
      }}
      state="ready"
    />,
  );
  expect(
    screen.queryByRole("button", { name: en["worklist.more"] }),
  ).toBeNull();
  expect(
    screen
      .getByRole("link", { name: en["brief.feed.fullWorklist"] })
      .getAttribute("href"),
  ).toBe("#/home?filter=all&queue=1");
});

it("warns about urgent work beyond the loaded page using server urgency facts", () => {
  stubApi({});
  const row = { ...taskRow("t", "Pinned task"), urgent: false };
  render(
    <BriefFeed
      day={{
        ...readingsDay({}, [row], [], { urgent: 3 }),
        focus: { items: [row], total: 4, urgent_remaining: 3 },
      }}
      state="ready"
    />,
  );
  expect(screen.getByText("3 more urgent items in the Worklist")).toBeTruthy();
});

it("shows dates and does not repeat the ranking comparator", () => {
  stubApi({});
  const row = {
    ...taskRow("t", "Call the buyer"),
    due_at: "2026-06-09T08:30:00Z",
  };
  render(<BriefFeed day={readingsDay({}, [row])} state="ready" />);
  expect(screen.getByText(/due 09\/06\/2026/)).toBeTruthy();
  expect(screen.queryByText("Why this is here")).toBeNull();
  expect(screen.queryByText(/Above the next/)).toBeNull();
});

it("does not claim an empty agenda when a source failed", () => {
  stubApi({});
  const day = readingsDay({}, []);
  day.sources_unavailable = [
    { source: "task", reason: "failed", category: "tasks" },
  ];
  const { container } = render(<BriefFeed day={day} state="ready" />);
  expect(screen.queryByText(en["brief.feed.clear"])).toBeNull();
  // The sentence pays the pane's inset, and no empty grid stands under it: a
  // list of nothing was only its own padding, a blank band under the caption.
  expect(
    screen.getByText(en["brief.feed.incomplete"]).closest(".panel-body"),
  ).not.toBeNull();
  expect(container.querySelector(".brief-feed-list")).toBeNull();
});

it("keeps approvals in the agenda and offers their review action", () => {
  stubApi({});
  const row = {
    ...taskRow("a", "Approve the buyer email"),
    source: "approval" as const,
    category: "decisions" as const,
    actions: ["decide" as const],
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  expect(inHand(container).getByText("Approve the buyer email")).toBeTruthy();
  expect(
    inHand(container).getByRole("button", { name: en["worklist.verb.decide"] }),
  ).toBeTruthy();
});

it.each([false, true])(
  "Focus omits personal pin controls (previously pinned: %s)",
  (pinned) => {
    stubApi({});
    const row: ReturnType<typeof taskRow> = {
      ...taskRow("task", "Call the buyer"),
      because: pinned ? [{ kind: "pinned" }] : [],
      above_next: pinned ? { comparator: "pin" } : undefined,
    };
    const { container } = render(
      <BriefFeed day={readingsDay({}, [row])} state="ready" />,
    );
    expect(
      screen.queryByRole("button", {
        name: en["worklist.verb.pin"],
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: en["worklist.verb.unpin"],
      }),
    ).toBeNull();
    expect(inHand(container).getByText("Call the buyer")).toBeTruthy();
    expect(screen.queryByText(/you pinned/i)).toBeNull();
  },
);

it("ends the row in hand's one line with the agent's move, every option beside it", () => {
  stubApi({});
  const { container } = render(
    <BriefFeed
      day={readingsDay({}, [waitingEmailRow()])}
      state="ready"
      onContext={() => {}}
    />,
  );
  const lead = inHand(container);
  const line = [
    ...(container
      .querySelector(".worklist-row-acts")
      ?.querySelectorAll("button, a") ?? []),
  ];
  // The put-downs lead from the far edge, in the open; the way in follows;
  // the move the product worked out closes the line, in the agent's chrome.
  expect(line.at(0)?.closest(".worklist-row-putdowns")).not.toBeNull();
  expect(
    lead
      .getByRole("button", { name: en["worklist.disposition.verb.not_mine"] })
      .closest(".worklist-row-putdowns"),
  ).not.toBeNull();
  expect(
    lead
      .getByRole("button", { name: en["brief.focus.context"] })
      .closest(".worklist-row-putdowns"),
  ).toBeNull();
  const draft = lead.getByRole("link", {
    name: en["worklist.verb.draft_reply_now"],
  });
  expect(draft.className).toContain("btn-ai");
  expect(line.at(-1)).toBe(draft);
  // The verb that only reaches the record is withheld: the record is named
  // and linked over the row.
  expect(
    lead.queryByRole("link", { name: en["worklist.verb.open"] }),
  ).toBeNull();
});

it("names whose row is in hand, and how the silence runs both ways", () => {
  stubApi({});
  const row = waitingEmailRow();
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  // Both moments off the row itself, no second read: the drawer draws the
  // same pair on every row, and a fetch per row is what this field ends.
  const about = container.querySelector(".brief-triage-about");
  expect(about?.textContent).toContain(en["worklist.pane.lastInbound"]);
  expect(about?.textContent).toContain(en["worklist.pane.lastOutbound"]);
  expect(about?.textContent).toContain("03/09/2026");
  expect(about?.textContent).toContain("28/08/2026");
  expect(
    screen.getByRole("link", { name: "Sonya Beck" }).getAttribute("href"),
  ).toBe("#/contacts/contact-sonya");
});

it("names the sender of a thread filed under a deal", () => {
  stubApi({});
  const row = {
    ...waitingEmailRow(),
    subject: { type: "deal" as const, id: "deal-retrofit", label: "Retrofit" },
    contact: { id: "contact-sonya", label: "Sonya Beck" },
  };
  render(<BriefFeed day={readingsDay({}, [row])} state="ready" />);
  expect(
    screen.getByRole("link", { name: "Sonya Beck" }).getAttribute("href"),
  ).toBe("#/contacts/contact-sonya");
});

it("claims no moments when the server withheld them", () => {
  stubApi({});
  const row = {
    ...waitingEmailRow(),
    contact: { id: "contact-sonya", label: "Sonya Beck" },
  };
  render(<BriefFeed day={readingsDay({}, [row])} state="ready" />);
  expect(screen.getByRole("link", { name: "Sonya Beck" })).toBeTruthy();
  expect(screen.queryByText(en["worklist.pane.lastInbound"])).toBeNull();
  expect(screen.queryByText(en["worklist.pane.never"])).toBeNull();
});

// THE RANKED COLUMN NAMES ITS ROWS FROM THE CONTACT, not from a message.
//
// Read off `email_summary.counterparty`, the line named the sender of a
// waiting message and left every other row anonymous — so a task the reader
// owes somebody stood in the column with its reason and nobody's name, while
// the same row in hand named them. The contact is the field every row carries.
it("names a task's contact in the ranked column", () => {
  stubApi({});
  const owed = {
    ...taskRow("owed", "Send the promised rollout comparison"),
    contact: {
      id: "contact-sonya",
      label: "Sonya Beck",
      touch: {
        last_inbound_at: "2026-09-03T16:46:00Z",
        last_outbound_at: "2026-08-28T09:12:00Z",
      },
    },
  };
  const { container } = render(
    <BriefFeed
      day={readingsDay({}, [waitingEmailRow(), owed])}
      onContext={() => undefined}
      state="ready"
      changed={undefined}
      refreshFailed={false}
      onRetry={() => undefined}
    />,
  );

  // THE TASK'S OWN ROW in the column, not the card and not the waiting message
  // above it: both of those named her before, and either would carry a
  // page-wide query over a line that still said nothing.
  const column = container.querySelector(".brief-triage-queue");
  if (!(column instanceof HTMLElement)) throw new Error("no ranked column");
  const row = within(column).getByRole("button", {
    name: /Send the promised rollout comparison/,
  });
  expect(within(row).getByText(/Sonya Beck/)).toBeTruthy();
});

// A WAITING ROW IS STILL NAMED BY ITS SUBJECT, from the row's own `title`.
//
// The server puts a waiting message's subject there (classify.go), so the
// column reads it like every other row rather than off `email_summary` — which
// is what kept the ranked list from drawing a part of a message outside the
// canonical row. The claim used to live in a helper's docblock with nothing
// holding it; this is what holds it.
it("names a waiting row by its subject", () => {
  stubApi({});
  const { container } = render(
    <BriefFeed
      day={readingsDay({}, [taskRow("t", "Call Weber"), waitingEmailRow()])}
      onContext={() => undefined}
      state="ready"
      changed={undefined}
      refreshFailed={false}
      onRetry={() => undefined}
    />,
  );

  const column = container.querySelector(".brief-triage-queue");
  if (!(column instanceof HTMLElement)) throw new Error("no ranked column");
  expect(
    within(column).getByRole("button", { name: /Meet next Tues\?/ }),
    "the column stopped naming a waiting row by the subject it is known by",
  ).toBeTruthy();
});
