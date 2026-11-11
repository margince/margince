/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { routeHash } from "../lib/router";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TaskNextSteps } from "./tasknextsteps";

type Activity = components["schemas"]["Activity"];

afterEach(() => {
  cleanup();
  globalThis.location.hash = "";
});

const TASK: Activity = {
  occurred_precision: "instant",
  offers_email: false,
  id: "task-1",
  kind: "task",
  subject: "Book the site visit with Alice",
  occurred_at: "2026-09-08T09:00:00Z",
  is_done: false,
  links: [
    { entity_type: "deal", entity_id: "d-1" },
    { entity_type: "contact", entity_id: "c-1" },
  ],
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-09-08T09:00:00Z",
  updated_at: "2026-09-08T09:00:00Z",
};

function show(task: Activity, routes: Parameters<typeof installFetchStub>[0]) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /contacts/c-1": () =>
      jsonResponse({ id: "c-1", full_name: "Alice Brandt", archived_at: null }),
    ...routes,
  });
  return render(
    <StoryProviders>
      <TaskNextSteps task={task} />
    </StoryProviders>,
  );
}

it("a meeting task opens the booking for the contact it is filed on", async () => {
  const user = userEvent.setup();
  show({ ...TASK, task_type: "meet" }, {});

  await user.click(
    await screen.findByRole("button", { name: en["scheduling.bookContact"] }),
  );
  expect(globalThis.location.hash).toBe(
    routeHash({ screen: "book", id: "contact-c-1" }),
  );
});

it("a review task opens the most specific record it is filed on", async () => {
  const user = userEvent.setup();
  show({ ...TASK, task_type: "review" }, {});

  await user.click(
    await screen.findByRole("button", { name: en["task.next.reviewDeal"] }),
  );
  expect(globalThis.location.hash).toBe(
    routeHash({ screen: "deals", id: "d-1" }),
  );
});

it("a task to do has no step: Done is the whole of it", async () => {
  show({ ...TASK, task_type: "do" }, {});

  expect(screen.queryByRole("button")).toBeNull();
  expect(screen.queryByRole("link")).toBeNull();
});

it("a deal question lists the open deals offered and files the mail on the one picked", async () => {
  const user = userEvent.setup();
  const relinks: unknown[] = [];
  const deal = (id: string, name: string, status: string) => () =>
    jsonResponse({
      id,
      name,
      status,
      amount_minor: 4_200_000,
      currency: "EUR",
      expected_close_date: "2026-12-15",
    });
  show(
    {
      ...TASK,
      subject: "Which deal is this mail about?",
      task_type: "decide",
      source_system: "which_deal",
      source_activity_id: "mail-1",
      raw: { offered_deal_ids: ["d-1", "d-2", "d-3"] },
    },
    {
      "GET /deals/d-1": deal("d-1", "Nordwind renewal 2027", "open"),
      "GET /deals/d-2": deal("d-2", "Nordwind pilot", "open"),
      "GET /deals/d-3": deal("d-3", "Nordwind 2025", "won"),
      "POST /activities/relink-thread": (body) => {
        relinks.push(body);
        return jsonResponse({});
      },
    },
  );

  await user.click(await screen.findByRole("radio", { name: /renewal 2027/ }));
  expect(screen.getByRole("radio", { name: /pilot/ })).toBeTruthy();
  expect(screen.queryByRole("radio", { name: /2025/ })).toBeNull();
  await user.click(
    screen.getByRole("button", { name: en["tasks.whichDeal.file"] }),
  );
  await waitFor(() => expect(relinks).toHaveLength(1));
  expect(relinks).toEqual([
    {
      activity_id: "mail-1",
      entity_type: "deal",
      entity_id: "d-1",
      replace_existing_of_type: false,
    },
  ]);
});
