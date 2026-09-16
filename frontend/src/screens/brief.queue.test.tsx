/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { BriefScreen } from "./brief";
import { readingsDay, taskRow } from "./brief.fixtures";
import { BriefQueue, WorklistRedirect } from "./brief.queue";
import { jsonResponse, render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
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

it("opens a review row's context without excluding it as non-selling work", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home?queue=1";
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
      name: /Show what 1, Review the disclosure/,
    }),
  );
  expect(
    screen.getByRole("complementary", { name: en["worklist.pane.title"] }),
  ).toBeTruthy();
  expect(window.location.hash).toContain("selected=notice_case-privacy");
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
  const focus = await screen.findByRole("region", { name: "Focus" });
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

it("opens a meeting's own record from its focus row", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home";
  const meeting: components["schemas"]["WorklistItem"] = {
    ...taskRow("held-meeting", "Discovery call with Turbinenbau"),
    source: "meeting_outcome",
    category: "meetings",
    actions: ["decide"],
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
  });
  render(<BriefScreen />);
  await user.click(
    await screen.findByRole("button", { name: en["brief.focus.context"] }),
  );
  expect(
    await screen.findByText("Organizer: Weber. Attendees: Weber, Ziethen."),
  ).toBeTruthy();
  // The row's own verbs answer a meeting; the read offers no "Done".
  expect(
    screen.queryByRole("button", { name: en["tasks.complete"] }),
  ).toBeNull();
});
