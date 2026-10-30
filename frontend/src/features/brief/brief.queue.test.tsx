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
import { BriefScreen } from "./brief";
import { BriefQueue, WorklistRedirect } from "./brief.queue";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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
  const focus = await screen.findByRole("region", { name: "First" });
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
      name: en["worklist.verb.meetingUpdateFor"].replace(
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
  expect(
    within(details).getByText(en["worklist.consequence.task_slips"]),
  ).toBeTruthy();
});

// THE BUTTON AND THE LIST IT OPENS NAME ONE QUEUE.
//
// The drawer keeps its scope in `queue_scope` and that survives the drawer
// closing, so a reader who switched it to the team and shut it had a button
// counting their own day over a list of somebody else's — two totals for one
// control, and the one on screen was the wrong one.
it("counts the queue the button actually opens", async () => {
  window.location.hash = "#/home?queue_scope=team";
  stubApi({
    "GET /worklist": (_body, query) =>
      jsonResponse(
        query.get("scope") === "team"
          ? {
              ...readingsDay({}, [], undefined, { total: 42 }),
              scope: "team",
              counts: [
                {
                  category: "tasks",
                  considered: 42,
                  shown: 0,
                  more_available: false,
                },
              ],
            }
          : readingsDay({}, [taskRow("t", "Call Weber")], undefined, {
              total: 3,
            }),
      ),
  });
  render(<BriefScreen />);

  // The team's total, not the reader's own three. Read off the count element
  // the switch carries rather than its accessible name, which is the verb
  // alone — `OptionCount` is beside the words, not in them.
  const toggle = await screen.findByRole("button", {
    name: en["brief.queue.show"],
  });
  await waitFor(() => {
    expect(
      toggle.querySelector(".optioncount")?.textContent,
      "the switch counted a different queue from the one it opens",
    ).toContain("42");
  });
});

it.each(["failed", "bounded", "scope"])(
  "does not claim an exact drawer total for %s coverage",
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
    const toggle = await screen.findByRole("button", {
      name: en["brief.queue.show"],
    });
    await waitFor(() =>
      expect(
        client.getQueryState(["worklist", "mine", "all", ""])?.status,
      ).toBe("success"),
    );
    expect(toggle.querySelector(".optioncount")).toBeNull();
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
