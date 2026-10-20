/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { BriefFeed } from "./brief.feed";
import {
  meetingRow,
  readingsDay,
  taskRow,
  waitingEmailRow,
} from "./brief.fixtures";
import { render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/** Whether a control's name is a row's menu, whatever the row is called. */
const MENU_LEAD = en["worklist.rowMenu"].slice(
  0,
  en["worklist.rowMenu"].indexOf("{title}"),
);
const rowMenu = (name: string) => name.startsWith(MENU_LEAD);

/** The queue beside the row in hand, one control per queued row. */
function inQueue(container: HTMLElement) {
  const queue = container.querySelector(".brief-triage-queue");
  if (!(queue instanceof HTMLElement)) throw new Error("no queue is drawn");
  return within(queue);
}

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
  // The day's first row is in hand, drawn whole. The queue beside it names
  // every focus row in the server's order. Each rank shows in the reader's
  // own numerals.
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
  await user.click(
    inQueue(container).getByRole("button", { name: /Call buyer 2/ }),
  );
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
    inQueue(container)
      .getByRole("button", { name: /Call buyer 2/ })
      .getAttribute("aria-pressed"),
  ).toBe("true");
});

it("neither grows focus nor links the whole queue when more pages exist", () => {
  stubApi({});
  const { container } = render(
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
  // The page header's "Show Worklist" button is Home's one way into the queue.
  expect(container.querySelector('a[href*="filter=all"]')).toBeNull();
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
  expect(
    screen
      .getByRole("link", { name: "3 more urgent items in the Worklist" })
      .getAttribute("href"),
  ).toBe("#/home?filter=urgent&queue=1");
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
  // The sentence pays the pane's inset, and no empty grid stands under it.
  // A list of nothing was only its own padding, a blank band under the caption.
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
  async (pinned) => {
    stubApi({});
    const row: ReturnType<typeof taskRow> = {
      ...taskRow("task", "Call the buyer"),
      because: pinned ? [{ kind: "pinned" }] : [],
      above_next: pinned ? { comparator: "pin" } : undefined,
    };
    const { container } = render(
      <BriefFeed day={readingsDay({}, [row])} state="ready" />,
    );
    // The pin would be a line of the row's menu, so the menu is opened first.
    await userEvent.click(
      inHand(container).getByRole("button", { name: rowMenu }),
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

it("ends the row in hand's one line with its menu, the agent's move before it", () => {
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
  // The way into what the row is about opens the line. Done stands before the
  // agent's move, and the row's menu, holding the other set-asides, closes it.
  expect(line.at(0)).toBe(
    lead.getByRole("button", { name: en["brief.focus.context"] }),
  );
  expect(line.at(-1)).toBe(lead.getByRole("button", { name: rowMenu }));
  const draft = lead.getByRole("link", {
    name: en["worklist.verb.draft_reply_now"],
  });
  expect(draft.className).toContain("btn-ai");
  expect(line.at(-2)).toBe(draft);
  expect(line.at(-3)).toBe(
    lead.getByRole("button", { name: en["worklist.disposition.verb.handled"] }),
  );
  expect(
    lead.queryByRole("button", {
      name: en["worklist.disposition.verb.not_mine"],
    }),
  ).toBeNull();
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

it("names the account a row in hand is about, and which side wrote last", () => {
  stubApi({});
  const row = {
    ...taskRow("renewal", "Send the renewal terms"),
    subject: {
      type: "company" as const,
      id: "company-nordwind",
      label: "Nordwind",
    },
    company: {
      id: "company-nordwind",
      touch: {
        last_inbound_at: "2026-09-03T09:00:00Z",
        last_outbound_at: null,
      },
    },
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  const about = container.querySelector(".brief-triage-about");
  expect(about?.querySelector("a")?.textContent).toBe("Nordwind");
  expect(about?.textContent).toContain(en["worklist.pane.lastFromCompany"]);
  expect(about?.textContent).toContain("03/09/2026");
  expect(about?.textContent).toContain(
    `${en["worklist.pane.lastToCompany"]} ${en["worklist.pane.never"]}`,
  );
  expect(about?.textContent).not.toContain(en["worklist.pane.lastInbound"]);
});

// The ranked column names its rows from the contact, not from a message.
//
// Read off `email_summary.counterparty`, the line left every row but a waiting
// message anonymous. The contact is the field every row carries.
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

  // The task's own row in the column, not the card or the waiting message
  // above it. Both named her already, so a page-wide query would pass anyway.
  const column = container.querySelector(".brief-triage-queue");
  if (!(column instanceof HTMLElement)) throw new Error("no ranked column");
  const row = within(column).getByRole("button", {
    name: /Send the promised rollout comparison/,
  });
  expect(within(row).getByText(/Sonya Beck/)).toBeTruthy();
});

// A waiting row is still named by its subject, from the row's own `title`.
//
// The server puts a waiting message's subject there (classify.go), so the
// column reads it like every other row. Reading `email_summary` would draw
// part of a message outside the canonical row.
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

it("names the host of a meeting in hand on Home's focus card", () => {
  stubApi({});
  const row = {
    ...meetingRow("m", false),
    host: {
      kind: "user" as const,
      id: "01a05500-0000-7000-8000-0000000000c3",
      label: "Lena Fischer",
    },
  };
  const { container } = render(
    <BriefFeed
      day={{
        ...readingsDay({}, [row]),
        focus: { items: [row], total: 1, urgent_remaining: 0 },
      }}
      state="ready"
    />,
  );
  const card = container.querySelector(".brief-triage-lead");
  expect(card?.textContent).toContain("hosted by Lena Fischer");
});
