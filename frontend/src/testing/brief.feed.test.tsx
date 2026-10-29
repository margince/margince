/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ToastProvider } from "../design-system/toast";
import { BriefFeed } from "../features/brief/brief.feed";
import { BriefGlance } from "../features/brief/brief.glance";
import { WorklistRow } from "../features/worklist/worklist.row";
import { en } from "../i18n/en";
import {
  failedApprovalRow,
  legacyStalledRow,
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
  // The day's first row is in hand, drawn whole. The list under it names
  // every other focus row in the server's order, with its rank in the
  // reader's own numerals.
  expect(
    [...container.querySelectorAll(".worklist-row-title")].map(
      (row) => row.textContent,
    ),
  ).toEqual([rows[0].title]);
  expect(
    [...container.querySelectorAll(".brief-focus-item-title")].map(
      (item) => item.textContent,
    ),
  ).toEqual(rows.slice(1, 6).map((row) => row.title));
  expect(
    [...container.querySelectorAll(".brief-focus-list > li")].map(
      (item) => item.querySelector(".brief-focus-rank")?.textContent,
    ),
  ).toEqual(["2", "3", "4", "5", "6"]);
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
  expect(screen.getByText("3 of 3")).toBeTruthy();
  // The pressed button is gone, so focus lands on the card it brought in hand.
  expect(
    document.activeElement?.contains(
      container.querySelector(".brief-triage-lead"),
    ),
  ).toBe(true);
  // The row in hand leaves the list; the rows still waiting keep their ranks.
  expect(
    inQueue(container).queryByRole("button", { name: /Call buyer 2/ }),
  ).toBeNull();
  expect(
    [...container.querySelectorAll(".brief-focus-list > li")].map(
      (item) => item.querySelector(".brief-focus-rank")?.textContent,
    ),
  ).toEqual(["1", "2"]);
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
  render(<BriefFeed day={day} state="ready" />);
  expect(screen.queryByText(en["brief.feed.clear"])).toBeNull();
  // The sentence stands in the section.
  expect(
    screen.getByText(en["brief.feed.incomplete"]).closest("#brief-today"),
  ).not.toBeNull();
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

it("leads the row in hand's one line with the agent's move, and ends it with its menu", () => {
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
  // The card is finished on its answer, so the answer opens the line. Then the
  // way into what the row is about and Done; the row's menu, holding the other
  // set-asides, closes it.
  const draft = lead.getByRole("link", {
    name: en["worklist.verb.draft_reply_now"],
  });
  expect(draft.className).toContain("btn-ai");
  expect(line.at(0)).toBe(draft);
  expect(line.at(1)).toBe(
    lead.getByRole("button", { name: en["brief.focus.context"] }),
  );
  expect(line.at(-2)).toBe(
    lead.getByRole("button", { name: en["worklist.disposition.verb.handled"] }),
  );
  expect(line.at(-1)).toBe(lead.getByRole("button", { name: rowMenu }));
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

it("names why the row in hand is first, whose it is, and its inbox", () => {
  stubApi({});
  const row = waitingEmailRow();
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  const head = container.querySelector(".brief-triage-lead-head");
  expect(head?.querySelector(".brief-triage-label-now")?.textContent).toBe(
    "Customer waiting",
  );
  expect(head?.textContent).toContain(en["visibility.team"]);
  expect(
    screen.getByRole("link", { name: "Sonya Beck" }).getAttribute("href"),
  ).toBe("/#/contacts/contact-sonya?from=worklist");
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
  ).toBe("/#/contacts/contact-sonya?from=worklist");
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

it("names the account a row in hand is about", () => {
  stubApi({});
  const row = {
    ...taskRow("renewal", "Send the renewal terms"),
    subject: {
      type: "company" as const,
      id: "company-nordwind",
      label: "Nordwind",
    },
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  const head = container.querySelector(".brief-triage-lead-head");
  expect(head?.querySelector("a")?.textContent).toBe("Nordwind");
});

// The ranked column names its rows from the contact, not from a message. A
// task carries a contact and no message, and must still show a name.
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

  // The task's own row in the column. The card and the waiting message both
  // name the contact too, so a page-wide query would prove nothing.
  const column = container.querySelector(".brief-triage-queue");
  if (!(column instanceof HTMLElement)) throw new Error("no ranked column");
  const row = within(column).getByRole("button", {
    name: /Send the promised rollout comparison/,
  });
  expect(within(row).getByText(/Sonya Beck/)).toBeTruthy();
});

// A waiting row is named by its subject, from the row's own `title`. The
// server puts the subject there, so the column never reads `email_summary`.
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

it("shows concrete reasons without ranking or consequence narration", () => {
  stubApi({});
  const { container } = render(
    <BriefFeed day={readingsDay({}, [waitingEmailRow()])} state="ready" />,
  );
  const card = inHand(container);
  expect(card.queryByText("Why first?")).toBeNull();
  expect(card.queryByText(/If ignored/)).toBeNull();
  expect(card.getByText(/waiting 13 days/)).toBeTruthy();
});

it.each([true, false])(
  "keeps action, failure and recovery separate from the hero (title %s)",
  (hasTitle) => {
    stubApi({});
    const failed = {
      ...failedApprovalRow(),
      title: hasTitle ? "Move deal to Won" : undefined,
    };
    const day = readingsDay({}, [failed]);
    const { container } = render(
      <ToastProvider>
        <BriefGlance
          view="morning"
          firstName="Ada"
          now={new Date("2026-09-03T08:00:00Z")}
          day={day}
          week={undefined}
        />
        <BriefFeed day={day} state="ready" />
      </ToastProvider>,
    );
    const hero = screen.getByTestId("brief-glance");
    expect(hero.textContent).not.toContain("Move deal to Won");
    expect(hero.textContent).not.toContain(failed.detail);
    const card = inHand(container);
    expect(container.querySelector(".worklist-row-title")?.textContent).toBe(
      hasTitle ? "Move deal to Won" : "Approved action did not run",
    );
    expect(card.getAllByText(failed.detail ?? "")).toHaveLength(1);
    expect(card.getAllByText("Approved but not run").length).toBeGreaterThan(0);
    expect(
      card.getByRole("button", { name: en["worklist.verb.retry"] }),
    ).toBeTruthy();
  },
);

it.each(["stalled", "closing_soon", "opportunity"])(
  "leads a legacy %s brief item with its deal instead of inventing an action",
  (kind) => {
    stubApi({});
    const row = { ...legacyStalledRow(), kind };
    const { container } = render(
      <BriefFeed day={readingsDay({}, [row])} state="ready" />,
    );
    const card = inHand(container);
    expect(container.querySelector(".worklist-row-title")?.textContent).toBe(
      "Acme Expansion",
    );
    expect(card.getByRole("heading", { name: "Acme Expansion" })).toBeTruthy();
    expect(container.querySelector(".worklist-row-context")).toBeNull();
    expect(card.getAllByText("Acme Expansion")).toHaveLength(1);
    expect(container.querySelector(".worklist-row-verdict")).toBeNull();
    expect(card.getByRole("button", { name: en["brief.act"] })).toBeTruthy();
    expect(
      card
        .getByRole("link", { name: en["deal.openDeal"] })
        .getAttribute("href"),
    ).toBe("/#/deals/deal-expansion");
  },
);

it("keeps the server's finding beneath the brief action", () => {
  stubApi({});
  const row = {
    ...legacyStalledRow(),
    verdict: {
      source: "brief_finding" as const,
      line: "No next meeting has been agreed.",
    },
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  expect(inHand(container).getAllByText(row.verdict.line)).toHaveLength(1);
  expect(container.querySelector(".worklist-row-title")?.textContent).toBe(
    "Acme Expansion",
  );
});

it("preserves a concrete brief action supplied by the server", () => {
  stubApi({});
  const row = {
    ...legacyStalledRow(),
    title: "Call the buyer about the renewal",
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  expect(container.querySelector(".worklist-row-title")?.textContent).toBe(
    "Acme Expansion",
  );
  expect(
    inHand(container).getByText("Call the buyer about the renewal"),
  ).toBeTruthy();
  expect(inHand(container).getAllByText("Acme Expansion")).toHaveLength(1);
});

// The defect was layout, and happy-dom lays nothing out. What holds is the
// mark: a task's name is a button, and only a button so marked may wrap.
it("marks a long task name in hand as data, so the headline wraps", () => {
  stubApi({});
  const title =
    "Call Alice back about the retrofit quote and the revised delivery window by noon";
  const { container } = render(
    <BriefFeed day={readingsDay({}, [taskRow("long", title)])} state="ready" />,
  );
  const name = inHand(container).getByRole("button", { name: title });
  expect(name.classList).toContain("btn-valuelabel");
});

it("keeps the canonical deal title for an unframed brief row", () => {
  stubApi({});
  const { container } = render(
    <WorklistRow item={legacyStalledRow()} position={1} owner="" />,
  );
  expect(container.querySelector(".worklist-row-title")?.textContent).toBe(
    "Acme Expansion",
  );
});

it("keeps three similar deal cards distinct and exposes commercial facts", async () => {
  stubApi({});
  const user = userEvent.setup();
  const rows = [
    {
      ...legacyStalledRow(),
      deal: {
        amount_minor: 2500000,
        currency: "EUR",
        close_date_provisional: false,
      },
    },
    {
      ...legacyStalledRow(),
      id: "renewal",
      subject: {
        type: "deal" as const,
        id: "deal-renewal",
        label: "Globex Renewal",
      },
      deal: {
        amount_minor: 1200000,
        currency: "EUR",
        close_date_provisional: false,
      },
    },
    {
      ...legacyStalledRow(),
      id: "rollout",
      subject: {
        type: "deal" as const,
        id: "deal-rollout",
        label: "Nordwind Rollout",
      },
      title: "Send the revised rollout proposal",
    },
  ];
  const { container } = render(
    <BriefFeed day={readingsDay({}, rows)} state="ready" />,
  );
  expect(
    inHand(container).getByRole("heading", { name: "Acme Expansion" }),
  ).toBeTruthy();
  expect(inHand(container).getByText("€25,000.00")).toBeTruthy();
  expect(container.querySelector(".worklist-row-context")).toBeNull();
  await user.click(
    inQueue(container).getByRole("button", { name: /Globex Renewal/ }),
  );
  expect(
    inHand(container).getByRole("heading", { name: "Globex Renewal" }),
  ).toBeTruthy();
  expect(inHand(container).getByText("€12,000.00")).toBeTruthy();
  await user.click(
    inQueue(container).getByRole("button", { name: /Nordwind Rollout/ }),
  );
  expect(
    inHand(container).getByRole("heading", { name: "Nordwind Rollout" }),
  ).toBeTruthy();
  expect(
    inHand(container).getByText("Send the revised rollout proposal"),
  ).toBeTruthy();
});

it("keeps overdue status out of the deal heading's accessible name", () => {
  stubApi({});
  const row = { ...legacyStalledRow(), overdue: true };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  expect(
    inHand(container).getByRole("heading", { name: "Acme Expansion" }),
  ).toBeTruthy();
  expect(inHand(container).getByText("Overdue")).toBeTruthy();
});

it("retains a differing source title as context without guessing a new action", () => {
  stubApi({});
  const row = {
    ...legacyStalledRow(),
    title: "Acme Expansion before rename",
    subject: {
      type: "deal" as const,
      id: "deal-expansion",
      label: "Acme Expansion 2026",
    },
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  expect(
    inHand(container).getByRole("heading", { name: "Acme Expansion 2026" }),
  ).toBeTruthy();
  expect(container.querySelector(".worklist-row-context")?.textContent).toBe(
    row.title,
  );
});

it("prints a supplied sentence once when the title repeats the finding", () => {
  stubApi({});
  const finding = "The buyer requested a revised proposal.";
  const row = {
    ...legacyStalledRow(),
    title: finding,
    verdict: { source: "brief_finding" as const, line: finding },
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  expect(inHand(container).getAllByText(finding)).toHaveLength(1);
  expect(container.querySelector(".worklist-row-context")).toBeNull();
});

it("opens a long task title without repeating its linked subject", () => {
  stubApi({});
  const title =
    "Call back about retrofit delivery and the revised installation schedule";
  const row = {
    ...taskRow("long-call", title),
    subject: { type: "contact" as const, id: "alice", label: "Alice" },
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  const card = inHand(container);
  const control = card.getByRole("button", { name: title });
  expect(control.classList.contains("btn-valuelabel")).toBe(true);
  expect(control.getAttribute("aria-haspopup")).toBe("dialog");
  expect(card.getAllByText("Alice")).toHaveLength(1);
});

it("keeps an unlinked subject in the framed title", () => {
  stubApi({});
  const row = {
    ...taskRow("unlinked", "Review the transcript"),
    subject: {
      type: "activity" as const,
      id: "recording",
      label: "Buyer meeting",
    },
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  expect(
    inHand(container).getByRole("button", {
      name: "Review the transcript · Buyer meeting",
    }),
  ).toBeTruthy();
});
