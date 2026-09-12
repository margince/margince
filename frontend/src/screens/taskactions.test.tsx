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
  stubTask("meeting-9");
  openTask();

  const open = await screen.findByRole("button", { name: "Open the meeting" });
  await userEvent.click(open);

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
  expect(screen.queryByRole("button", { name: "Open the meeting" })).toBeNull();
});
