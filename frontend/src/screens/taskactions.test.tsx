/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test } from "vitest";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { TaskDetailModal } from "./taskactions";

afterEach(cleanup);

// The provenance sentence and the way back to the record behind it.
//
// A task read out of a meeting says so in words — "committed to this in the
// meeting transcript (line 6)" — and for as long as that sentence was all it
// carried, the only route to the meeting was the record's history tab and a
// search for its exact subject. These hold the link, and hold that a task
// nobody read out of anything does not offer one.

function stubTask(sourceActivityId: string | null) {
  installFetchStub({
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Create a comparison sheet with two products",
        body: "Lena Fischer committed to this in the meeting transcript (line 6).",
        due_at: "2026-09-11T21:59:59Z",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        source_activity_id: sourceActivityId,
      }),
    // The PLAIN activity read, which is what the reader calls. Stubbing the
    // email presentation instead is what let a 404 ship: that endpoint refuses
    // any kind but `email`, and a fixture answering it anyway proved only that
    // the fixture answered.
    "GET /activities/meeting-9": () =>
      jsonResponse({
        id: "meeting-9",
        kind: "meeting",
        subject: "Akeneo — Vergleichsblatt",
        body: "Lena: Ich mache ein Vergleichsblatt mit zwei Produkten.",
        occurred_at: "2026-09-08T08:00:00Z",
        source: "transcript",
        captured_by: "human:u-1",
        version: 1,
      }),
  });
}

function openTask() {
  return render(
    <StoryProviders>
      <TaskDetailModal
        activityId="task-1"
        readOnly={false}
        onClose={() => {}}
        update={
          {
            mutate: () => {},
            isPending: false,
          } as unknown as Parameters<typeof TaskDetailModal>[0]["update"]
        }
      />
    </StoryProviders>,
  );
}

test("a task read out of a meeting opens that meeting", async () => {
  const user = userEvent.setup();
  stubTask("meeting-9");
  openTask();

  const open = await screen.findByRole("button", { name: "Open original" });
  await user.click(open);

  // The meeting itself, not another copy of the sentence naming it.
  await waitFor(() => {
    expect(screen.getByText("Akeneo — Vergleichsblatt")).not.toBeNull();
  });
});

test("a task nobody read out of a meeting offers no way back", async () => {
  stubTask(null);
  openTask();

  // The positive control is the test above: without it a build that never
  // rendered the button would pass this one for the wrong reason.
  await screen.findByText(/committed to this in the meeting transcript/);
  expect(screen.queryByRole("button", { name: "Open original" })).toBeNull();
});

test("a transcript refused on reopen is not painted from the cache", async () => {
  const user = userEvent.setup();
  // The kind lookup and the reader share one query key, and the lookup's
  // observer outlives the drawer — so `gcTime: 0` cannot evict between the
  // first read and the second, and the refused reopen has a cached transcript
  // sitting in `data`. Rendering that beside the error is what this holds.
  let refused = false;
  installFetchStub({
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Create a comparison sheet with two products",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        source_activity_id: "meeting-9",
      }),
    "GET /activities/meeting-9": () =>
      refused
        ? jsonResponse({ title: "Not found", status: 404 }, 404)
        : jsonResponse({
            id: "meeting-9",
            kind: "meeting",
            subject: "Akeneo — Vergleichsblatt",
            body: "Lena: Ich mache ein Vergleichsblatt mit zwei Produkten.",
            occurred_at: "2026-09-08T08:00:00Z",
            version: 1,
          }),
  });
  openTask();

  await user.click(
    await screen.findByRole("button", { name: "Open original" }),
  );
  await screen.findByText(/Vergleichsblatt mit zwei Produkten/);

  // Access goes away, and the reader is opened again. Escape rather than the
  // close control: the task drawer and the transcript reader are both dialogs
  // and both draw a "Close", so naming one finds two.
  refused = true;
  await user.keyboard("{Escape}");
  await user.click(
    await screen.findByRole("button", { name: "Open original" }),
  );

  // The words are gone, not merely accompanied by an error.
  await waitFor(() => {
    expect(screen.queryByText(/Vergleichsblatt mit zwei Produkten/)).toBeNull();
  });
});

test("a source that cannot be read says so instead of disappearing", async () => {
  installFetchStub({
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Create a comparison sheet with two products",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        source_activity_id: "meeting-9",
      }),
    // The kind lookup fails, so nothing downstream knows which reader to be.
    "GET /activities/meeting-9": () =>
      jsonResponse({ title: "Server error", status: 500 }, 500),
  });
  openTask();

  // A retry, rather than a panel that silently removed the evidence: the email
  // panel's own failure arm cannot speak here, because it never mounts.
  expect(await screen.findByRole("button", { name: "Try again" })).toBeTruthy();
});

test("an email request is read in the task, with no click to open it", async () => {
  installFetchStub({
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Report requested",
        source_activity_id: "email-1",
        occurred_at: "2026-09-01T09:00:00Z",
        is_done: false,
      }),
    "GET /activities/email-1": () =>
      jsonResponse({
        id: "email-1",
        kind: "email",
        body: "Unparsed body must not be displayed",
      }),
    "GET /activities/email-1/email-presentation": () =>
      jsonResponse({
        id: "email-1",
        lifecycle: "delivered",
        occurred_at: "2026-09-01T09:00:00Z",
        version: 1,
        summary: {
          activity_id: "email-1",
          occurred_at: "2026-09-01T09:00:00Z",
          version: 1,
          subject: "The original request",
          display_status: "team",
          move: "needs_reply",
          attachment_count: 0,
        },
        body: "Please send the report.",
        from: [],
        to: [],
        cc: [],
        bcc: [],
        bcc_withheld: false,
        attachments: [],
        links: [],
        thread: { members: [], next_cursor: null },
        can_reply: false,
        can_relink: false,
        access: {
          content_state: "available",
          display_status: "team",
          audience: "workspace",
          can_change: false,
          change_mode: "none",
        },
      }),
  });
  openTask();

  // The message itself, with nothing pressed. It used to sit behind "Open
  // original", which put a second drawer over the task and cost a click to see
  // the one fact the task rests on.
  expect(await screen.findByText("Please send the report.")).toBeTruthy();
  // The presentation's body, never the raw activity's — that endpoint is the
  // normalised read, and the plain activity carries an unparsed copy.
  expect(screen.queryByText("Unparsed body must not be displayed")).toBeNull();
  // And no button, because there is nothing left for one to open.
  expect(screen.queryByRole("button", { name: "Open original" })).toBeNull();

  // WHERE it sits is the requirement, not merely that it is present: the
  // message reads under the task's own verbs, so a mail long enough to scroll
  // cannot push the Done button and the date picker off the panel.
  const moveTo = screen.getByText("Move to");
  const message = screen.getByText("Please send the report.");
  expect(
    moveTo.compareDocumentPosition(message) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});
