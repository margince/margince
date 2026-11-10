/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../../lib/storage";
import {
  failedApprovalRow,
  readingsDay,
  taskRow,
} from "../../testing/brief.fixtures";
import {
  emptyPage,
  jsonResponse,
  render,
  stubApi,
} from "../../testing/brief.testkit";
import { meFixture } from "../../testing/mefixture";
import { BriefScreen } from "./brief";
import { BriefQueue, WorklistRedirect } from "./brief.queue";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
  removeStored(STORAGE_KEYS.worklistDrawerWide);
});

it("redirects legacy owner links without losing their filter or email", async () => {
  window.location.hash =
    "#/worklist/colleague?filter=changed_since_brief&email=message";
  render(<WorklistRedirect opensOn="colleague" />);
  await waitFor(() =>
    expect(window.location.hash).toBe(
      "#/home?email=message&filter=changed_since_brief&owner=colleague&queue=1",
    ),
  );
});

it("keeps queue scope separate from the weekly team brief and closes with Escape", async () => {
  const user = userEvent.setup();
  window.location.hash =
    "#/home?view=weekly&scope=team&queue=1&queue_scope=unassigned";
  const scopes: (string | null)[] = [];
  stubApi({
    "GET /worklist": (_body, query) => {
      scopes.push(query.get("scope"));
      return jsonResponse({ ...readingsDay({}, []), scope: "unassigned" });
    },
  });
  render(<BriefQueue />);
  await screen.findByRole("dialog", { name: en["brief.queue.title"] });
  await waitFor(() => expect(scopes).toContain("unassigned"));
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(window.location.hash).toContain("scope=team");
  expect(window.location.hash).toContain("view=weekly");
  expect(window.location.hash).not.toContain("queue=1");
});

it("opens as wide as this browser last left it, and remembers each press", async () => {
  const user = userEvent.setup();
  writeStored(STORAGE_KEYS.worklistDrawerWide, "1");
  window.location.hash = "#/home?queue=1";
  stubApi({
    "GET /worklist": () =>
      jsonResponse(readingsDay({}, [taskRow("t", "Call Weber")])),
  });
  render(<BriefQueue />);
  const drawer = await screen.findByRole("dialog", {
    name: en["brief.queue.title"],
  });
  expect(drawer.classList.contains("modal-drawer-window")).toBe(true);

  await user.click(screen.getByRole("button", { name: en["common.collapse"] }));
  await waitFor(() =>
    expect(drawer.classList.contains("modal-drawer-window")).toBe(false),
  );
  expect(readStored(STORAGE_KEYS.worklistDrawerWide)).toBe("0");

  await user.click(screen.getByRole("button", { name: en["common.expand"] }));
  await waitFor(() =>
    expect(drawer.classList.contains("modal-drawer-window")).toBe(true),
  );
  expect(readStored(STORAGE_KEYS.worklistDrawerWide)).toBe("1");
  // The address stays the queue's alone, so a shared link carries no layout.
  expect(window.location.hash).toBe("#/home?queue=1");
});

it("restores the queue opener's keyboard focus after closing", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home";
  stubApi({
    "GET /worklist": () =>
      jsonResponse(readingsDay({}, [taskRow("t", "Call Weber")])),
  });
  render(<BriefScreen />);
  await screen.findAllByText("Call Weber");
  const opener = screen.getByRole("button", {
    name: new RegExp(en["brief.queue.show"]),
  });
  await user.click(opener);
  await screen.findByRole("dialog", { name: en["brief.queue.title"] });
  await user.click(screen.getByRole("button", { name: /close/i }));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(opener);
});

// A review row is not excluded as non-selling work: it can be taken in hand
// like any other. What it opens INTO differs by surface — the queue's own page
// draws the record's pane beside it (worklist.detail.test.tsx), the drawer
// draws none, because the row already names whom it is about and a drawer has
// no third of its width to spend saying it twice.
it("takes a review row in hand, and opens no column beside it", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home?queue=1&queue_view=system";
  const review: components["schemas"]["WorklistItem"] = {
    ...taskRow("privacy", "Review the disclosure"),
    source: "notice_case",
    category: "system",
    destination: "review",
    subject: { type: "contact", id: "contact", label: "Alice" },
  };
  stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [review])),
    "GET /contacts/contact/360": () =>
      jsonResponse({ type: "not_found", title: "Not found", status: 404 }, 404),
  });
  render(<BriefQueue />);
  await user.click(
    await screen.findByRole("button", {
      name: /Show details for 1, Review the disclosure/,
    }),
  );
  expect(window.location.hash).toContain("selected=notice_case-privacy");
  expect(
    screen.queryByRole("complementary", { name: en["worklist.pane.title"] }),
    "the drawer spent a third of its width repeating the row above it",
  ).toBeNull();
});

it("opens a task's evidence without replacing the Home overview", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home";
  const task = taskRow("evidence-task", "Prepare the comparison");
  stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [task])),
    "GET /activities/evidence-task": () =>
      jsonResponse({
        id: task.id,
        kind: "task",
        subject: task.title,
        body: "Original commitment: prepare two variants by Friday.",
        occurred_at: "2026-09-01T10:00:00Z",
        version: 1,
        is_done: false,
      }),
  });
  render(<BriefScreen />);
  const focus = await screen.findByRole("region", {
    name: en["brief.first.title"],
  });
  const opener = await screen.findByRole("button", {
    name: en["brief.focus.context"],
  });
  await user.click(opener);
  expect(
    await screen.findByText(
      "Original commitment: prepare two variants by Friday.",
    ),
  ).toBeTruthy();
  expect(focus.isConnected).toBe(true);
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(document.activeElement).toBe(opener);
});

// A meeting whose outcome is missing opens the meeting itself. What the row
// asks and the row's own verbs sit above it, so the reader answers it there.
it("opens a meeting on what it asks, with the row's verbs", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home";
  const meeting: components["schemas"]["WorklistItem"] = {
    ...taskRow("held-meeting", "Discovery call with Turbinenbau"),
    source: "meeting_outcome",
    category: "meetings",
    because: [{ kind: "outcome_unrecorded" }],
    actions: ["decide", "cancel"],
  };
  stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [meeting])),
    "GET /activities/held-meeting": () =>
      jsonResponse({
        id: meeting.id,
        kind: "meeting",
        subject: meeting.title,
        body: "Organizer: Weber. Attendees: Weber, Ziethen.",
        occurred_at: "2026-09-01T10:00:00Z",
        version: 1,
        is_done: false,
      }),
    // The meeting reader's own reads: its files, follow-ups and field labels.
    "GET /activities": () => jsonResponse(emptyPage),
    "GET /attachments": () => jsonResponse(emptyPage),
    "GET /custom-fields": () => jsonResponse(emptyPage),
    "GET /installation/settings": () => jsonResponse({}),
  });
  render(<BriefScreen />);
  // The card in hand says what the row asks before anything is opened.
  expect(
    await screen.findByText(en["worklist.ask.meeting_outcome"]),
  ).toBeTruthy();
  await user.click(
    await screen.findByRole("button", { name: en["brief.focus.context"] }),
  );
  const details = await screen.findByRole("dialog");
  expect(
    await within(details).findByText(
      "Organizer: Weber. Attendees: Weber, Ziethen.",
    ),
  ).toBeTruthy();
  expect(
    within(details).getByText(en["worklist.ask.meeting_outcome"]),
  ).toBeTruthy();
  expect(
    within(details).getByRole("button", {
      name: en["worklist.verb.meetingAddNotesFor"].replace(
        "{subject}",
        "Discovery call with Turbinenbau",
      ),
    }),
  ).toBeTruthy();
  expect(
    within(details).getByRole("button", {
      name: en["worklist.verb.meetingCanceledFor"].replace(
        "{subject}",
        "Discovery call with Turbinenbau",
      ),
    }),
  ).toBeTruthy();
  // The row's own verbs answer a meeting; the read offers no "Done".
  expect(
    screen.queryByRole("button", { name: en["tasks.complete"] }),
  ).toBeNull();
});

// A row about a deal has no contact pane, and still opens on why it is here.
// That is its reasons and what slips if nobody acts; the row line has no room.
it("opens a deal row on why it needs attention", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home";
  stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [failedApprovalRow()])),
    "GET /deals/deal-nordwind": () =>
      jsonResponse({
        id: "deal-nordwind",
        name: "Nordwind renewal",
        amount_minor: 4_200_000,
        currency: "EUR",
        pipeline_id: "pl",
        stage_id: "s-proposal",
        status: "open",
        source: "manual",
        created_at: "2026-07-06T08:00:00Z",
        updated_at: "2026-07-06T08:00:00Z",
      }),
    "GET /pipelines": () =>
      jsonResponse({
        data: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages: [
              {
                id: "s-proposal",
                pipeline_id: "pl",
                name: "Proposal",
                position: 2,
                semantic: "open",
                win_probability: 40,
              },
            ],
          },
        ],
        page: { next_cursor: null },
      }),
  });
  render(<BriefScreen />);
  await user.click(
    await screen.findByRole("button", { name: en["brief.focus.context"] }),
  );
  const details = await screen.findByRole("dialog", {
    name: en["brief.focus.context"],
  });
  expect(within(details).getByText(en["worklist.why.title"])).toBeTruthy();
  expect(
    within(details).getByText(en["worklist.because.approved_and_failed"]),
  ).toBeTruthy();
  // The deal it is about: worth, and where it stands.
  expect(await within(details).findByText("€42,000")).toBeTruthy();
  expect(await within(details).findByText("Proposal")).toBeTruthy();
});

// A follow-up opens on the messages themselves, not on a restatement of why.
it("opens a follow-up on the conversation it waits on", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home";
  const followUp = {
    ...failedApprovalRow(),
    id: "msg-quote",
    source: "awaiting_reply",
    title: "Re: phase 2 pricing",
    subject: undefined,
    because: [{ kind: "you_wrote_last" }],
    consequence: "none",
    actions: ["open"],
    email_summary: {
      activity_id: "msg-quote",
      occurred_at: "2026-09-01T09:00:00Z",
      occurred_precision: "instant",
      version: 1,
      subject: "Re: phase 2 pricing",
      preview: "Attached is the quote for both phases.",
      direction: "outbound",
      display_status: "team",
      move: "waiting_for_them",
      attachment_count: 0,
    },
  } satisfies components["schemas"]["WorklistItem"];
  stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [followUp])),
    "GET /activities/msg-quote/email-presentation": () =>
      jsonResponse({
        id: "msg-quote",
        lifecycle: "delivered",
        occurred_at: "2026-09-01T09:00:00Z",
        summary: followUp.email_summary,
        body: "Attached is the quote for both phases.",
        from: [{ address: "rep@demo.test", display_name: "Rep" }],
        to: [],
        cc: [],
        bcc: [],
        bcc_withheld: false,
        attachments: [],
        links: [],
        thread: { members: [], next_cursor: null },
        access: {
          content_state: "available",
          audience: "workspace",
          selected_members: [],
          display_status: "team",
          can_change: false,
          change_mode: "message",
          held_by_others: false,
        },
        can_reply: true,
        can_forward: true,
        can_relink: false,
        version: 1,
      }),
  });
  render(<BriefScreen />);
  await user.click(
    await screen.findByRole("button", { name: en["brief.focus.context"] }),
  );
  const details = await screen.findByRole("dialog", {
    name: en["brief.focus.context"],
  });
  expect(
    within(details).getByText(en["worklist.pane.conversation"]),
  ).toBeTruthy();
  expect(
    await within(details).findByText("Attached is the quote for both phases."),
  ).toBeTruthy();
});

// THE BUTTON AND THE LIST IT OPENS NAME ONE QUEUE.
//
// The drawer keeps its scope in `queue_scope` and that survives the drawer
// closing, so a reader who switched it to the team and shut it had a button
// counting their own day over a list of somebody else's — two totals for one
// control, and the one on screen was the wrong one.
it("opens the reader's own Today and counts that list, whatever the drawer was last left on", async () => {
  window.location.hash =
    "#/home?queue_scope=team&queue_view=decisions&filter=tasks";
  stubApi({
    "GET /worklist": (_body, query) =>
      jsonResponse(
        query.get("scope") === "team"
          ? {
              ...readingsDay({}, [], undefined, { total: 42 }),
              scope: "team",
            }
          : {
              ...readingsDay({}, [taskRow("t", "Call Weber")]),
              counts: [
                {
                  category: "tasks",
                  considered: 3,
                  shown: 1,
                  more_available: true,
                },
              ],
            },
      ),
  });
  const user = userEvent.setup();
  render(<BriefScreen />);

  // The reader's own three, the list the button opens, not the team's 42 a
  // closed drawer was last narrowed to. Read off the count the switch
  // carries, which sits beside its words rather than in them.
  const toggle = await screen.findByRole("button", {
    name: en["brief.queue.show"],
  });
  await waitFor(() => {
    expect(toggle.querySelector(".optioncount")?.textContent).toContain("3");
  });
  await user.click(toggle);
  const hash = window.location.hash;
  expect(hash).toContain("queue=1");
  expect(hash).not.toContain("queue_scope");
  expect(hash).not.toContain("queue_view");
  expect(hash).not.toContain("filter=");
});

it("counts a closed drawer over the reader's own queue from Home's one read", async () => {
  window.location.hash = "#/home";
  const views: (string | null)[] = [];
  stubApi({
    "GET /worklist": (_body, query) => {
      views.push(query.get("view"));
      return jsonResponse({
        ...readingsDay({}, [taskRow("t", "Call Weber")]),
        counts: [
          { category: "tasks", considered: 7, shown: 1, more_available: false },
          {
            category: "system",
            considered: 5,
            shown: 0,
            more_available: false,
          },
        ],
      });
    },
  });
  render(<BriefScreen />);

  // Today's total leaves the System rows out, as the drawer's head does.
  const toggle = await screen.findByRole("button", {
    name: en["brief.queue.show"],
  });
  await waitFor(() =>
    expect(toggle.querySelector(".optioncount")?.textContent).toContain("7"),
  );
  expect(views, "a closed drawer sent a second Worklist read").toEqual([
    "today",
  ]);
});

it("reads the day once when Home opens with the drawer open", async () => {
  window.location.hash = "#/home?queue=1";
  const views: (string | null)[] = [];
  stubApi({
    "GET /worklist": (_body, query) => {
      views.push(query.get("view"));
      return jsonResponse(readingsDay({}, [taskRow("t", "Call Weber")]));
    },
  });
  render(<BriefScreen />);

  await screen.findByRole("dialog", { name: en["brief.queue.title"] });
  await screen.findAllByText("Call Weber");
  expect(views, "Home and its drawer each read the day").toEqual(["today"]);
});

it("keeps a closed drawer's count on Home's live read after the drawer was open", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home";
  let tasks = 7;
  stubApi({
    "GET /worklist": () =>
      jsonResponse({
        ...readingsDay({}, [taskRow("t", "Call Weber")]),
        counts: [
          {
            category: "tasks",
            considered: tasks,
            shown: 1,
            more_available: false,
          },
        ],
      }),
  });
  const { client } = render(<BriefScreen />);
  const count = () =>
    screen
      .getByRole("button", { name: new RegExp(en["brief.queue.show"]) })
      .querySelector(".optioncount")?.textContent;
  await waitFor(() => expect(count()).toContain("7"));

  await user.click(
    screen.getByRole("button", { name: new RegExp(en["brief.queue.show"]) }),
  );
  await screen.findByRole("dialog", { name: en["brief.queue.title"] });
  await user.click(screen.getByRole("button", { name: /close/i }));
  tasks = 9;
  await client.refetchQueries({
    queryKey: ["worklist", "mine", "all", "", "today"],
    exact: true,
  });

  await waitFor(() =>
    expect(count(), "the closed drawer kept its own stale day").toContain("9"),
  );
});

it.each(["failed", "bounded", "scope"])(
  "draws the drawer total for %s coverage as a floor",
  async (gap) => {
    const day = readingsDay({}, [], undefined, { total: 42 });
    day.counts = [
      {
        category: "tasks",
        considered: 42,
        shown: 0,
        more_available: gap === "bounded",
      },
    ];
    if (gap === "failed")
      day.sources_unavailable = [
        { source: "task", reason: "failed", category: "tasks" },
      ];
    if (gap === "scope") day.scope_truncated = true;
    stubApi({ "GET /worklist": () => jsonResponse(day) });
    const { client } = render(<BriefScreen />);
    await waitFor(() =>
      expect(
        client.getQueryState(["worklist", "mine", "all", "", "today"])?.status,
      ).toBe("success"),
    );
    // The tabs draw the same "42+", so the way into the queue agrees with them.
    const toggle = await screen.findByRole("button", {
      name: new RegExp(en["brief.queue.show"]),
    });
    await waitFor(() =>
      expect(toggle.querySelector(".optioncount")?.textContent).toBe("42+"),
    );
  },
);

it("keeps optional reconnections reachable in the real Home queue drawer", async () => {
  window.location.hash = "#/home?queue=1";
  stubApi({
    "GET /worklist": () =>
      jsonResponse({
        ...readingsDay({}, []),
        optional_relationships: [
          {
            ...taskRow(
              "01930000-0000-7000-8000-000000000031",
              "Demo reconnection",
            ),
            source: "relationship_decay",
            category: "relationships",
            level: 6,
            actions: ["open", "dismiss"],
            because: [],
          },
        ],
      }),
  });
  render(<BriefQueue />);
  const drawer = await screen.findByRole("dialog", {
    name: en["brief.queue.title"],
  });
  expect(
    await within(drawer).findByText(en["worklist.relationships.optional"]),
  ).toBeVisible();
  expect(within(drawer).getByText("Demo reconnection")).toBeVisible();
});

it("reads optional reconnections only when the reader opens them", async () => {
  window.location.hash = "#/home?queue=1";
  const asked: string[] = [];
  stubApi({
    "GET /worklist": (_body, query) => {
      asked.push(query.get("relationships") ?? "");
      if (query.get("relationships") === "omit") {
        return jsonResponse(readingsDay({}, []));
      }
      return jsonResponse({
        ...readingsDay({}, []),
        optional_relationships: [
          {
            ...taskRow("01930000-0000-7000-8000-000000000032", "Quiet buyer"),
            source: "relationship_decay",
            category: "relationships",
            level: 6,
            actions: ["open", "dismiss"],
            because: [],
          },
        ],
      });
    },
  });
  render(<BriefQueue />);
  const drawer = await screen.findByRole("dialog", {
    name: en["brief.queue.title"],
  });
  const show = await within(drawer).findByRole("button", {
    name: en["worklist.relationships.show"],
  });
  expect(asked).not.toContain("include");
  await userEvent.click(show);
  expect(await within(drawer).findByText("Quiet buyer")).toBeVisible();
  expect(asked.filter((value) => value === "include")).toHaveLength(1);
});

it("snoozes the Home task before a slow day refresh and pins the next click to its new version", async () => {
  vi.setSystemTime(new Date("2026-11-05T09:00:00Z"));
  const user = userEvent.setup();
  window.location.hash = "#/home";
  let version = 1;
  let dueAt = "2026-10-24T14:00:00Z";
  let dayReads = 0;
  let patches = 0;
  let releasePatch: () => void = () => {};
  const waitingPatch = new Promise<void>((resolve) => {
    releasePatch = resolve;
  });
  const task = () => ({
    id: "t",
    kind: "task",
    subject: "Call the buyer",
    writable: true,
    version,
    due_at: dueAt,
    due_has_time: true,
    is_done: false,
    occurred_at: "2026-10-24T09:00:00Z",
  });
  let releaseDay: (reply: Response) => void = () => {};
  const waitingDay = new Promise<Response>((resolve) => {
    releaseDay = resolve;
  });
  const row = {
    ...taskRow("t", "Call the buyer"),
    due_at: dueAt,
    due_has_time: true,
    version: 1,
    overdue: true,
  };
  const day = readingsDay({}, [row]);
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { activity: ["read", "update"] } })),
    "GET /worklist": () => (++dayReads === 1 ? jsonResponse(day) : waitingDay),
    "GET /activities/t": () => jsonResponse(task()),
    "PATCH /activities/t": async (body) => {
      if (
        typeof body !== "object" ||
        body === null ||
        !("due_at" in body) ||
        typeof body.due_at !== "string"
      ) {
        throw new Error("Expected a due date patch");
      }
      patches += 1;
      if (patches === 1) await waitingPatch;
      version += 1;
      dueAt = body.due_at;
      return jsonResponse(task());
    },
  });
  const { client } = render(<BriefScreen />);
  await user.click(
    await screen.findByRole("button", { name: en["brief.focus.context"] }),
  );
  const dialog = await screen.findByRole("dialog");
  const snooze = await within(dialog).findByRole("button", {
    name: en["tasks.snooze"],
  });
  await user.click(snooze);
  await user.click(snooze);
  expect(patches).toBe(1);
  expect(snooze).toBeDisabled();
  releasePatch();
  await waitFor(() => expect(snooze).toBeEnabled());
  const card = screen.getByRole("article");
  await waitFor(() =>
    expect(within(card).queryByText(en["worklist.overdue"])).toBeNull(),
  );
  expect(dueAt).toBe("2026-11-06T14:00:00.000Z");
  expect(client.isFetching({ queryKey: ["worklist"] })).toBeGreaterThan(0);
  const cached = client.getQueryData<{
    pages: components["schemas"]["Worklist"][];
  }>(["worklist", "mine", "all", "", "today"]);
  expect(cached?.pages[0]?.focus?.items[0]?.due_at).toBe(dueAt);
  expect(cached?.pages[0]?.focus?.items[0]?.version).toBe(2);
  await user.click(snooze);
  await waitFor(() => expect(patches).toBe(2));
  const sentVersions = vi
    .mocked(fetch)
    .mock.calls.flatMap(([request]) =>
      request instanceof Request && request.method === "PATCH"
        ? [request.headers.get("If-Match")]
        : [],
    );
  expect(sentVersions).toEqual(["1", "2"]);
  expect(dueAt).toBe("2026-11-07T14:00:00.000Z");
  releaseDay(jsonResponse(day));
  vi.useRealTimers();
});
