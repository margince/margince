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

it("a meeting task links to the booking for the contact it is filed on", async () => {
  const user = userEvent.setup();
  show({ ...TASK, task_type: "meet" }, {});
  const book = routeHash({ screen: "book", id: "contact-c-1" });

  const link = await screen.findByRole("link", {
    name: en["scheduling.bookContact"],
  });
  expect(link.getAttribute("href")).toContain(book);
  await user.click(link);
  expect(globalThis.location.hash).toBe(book);
});

it("a review task links to the most specific record it is filed on", async () => {
  const user = userEvent.setup();
  show({ ...TASK, task_type: "review" }, {});
  const deal = routeHash({ screen: "deals", id: "d-1" });

  const link = await screen.findByRole("link", {
    name: en["task.next.reviewDeal"],
  });
  expect(link.getAttribute("href")).toContain(deal);
  await user.click(link);
  expect(globalThis.location.hash).toBe(deal);
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

it("a deal question offers no answer until every deal offered has been read", async () => {
  const user = userEvent.setup();
  let failing = true;
  const deal = (id: string, name: string) =>
    jsonResponse({ id, name, status: "open", currency: "EUR" });
  show(
    {
      ...TASK,
      task_type: "decide",
      source_system: "which_deal",
      source_activity_id: "mail-1",
      raw: { offered_deal_ids: ["d-1", "d-2"] },
    },
    {
      "GET /deals/d-1": () => deal("d-1", "Nordwind renewal 2027"),
      "GET /deals/d-2": () =>
        failing
          ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
          : deal("d-2", "Nordwind pilot"),
    },
  );

  const retry = await screen.findByRole("button", { name: en["common.retry"] });
  expect(screen.queryByRole("radio")).toBeNull();
  expect(
    screen.queryByRole("button", { name: en["tasks.whichDeal.none"] }),
  ).toBeNull();
  failing = false;
  await user.click(retry);
  expect(await screen.findByRole("radio", { name: /pilot/ })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: en["tasks.whichDeal.none"] }),
  ).toBeTruthy();
});
