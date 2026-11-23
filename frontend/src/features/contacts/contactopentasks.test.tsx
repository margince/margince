// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { ViewerZoneProvider } from "../../lib/viewerzone";
import { FIXTURE_TIMEZONE } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
} from "../../testing/story-utils";
import { useContact360 } from "./contact360";
import { ContactOpenTasks } from "./contactopentasks";

// The Open tasks card under the contact's Next bar lists every open task the
// bar does not already draw. A tick shows only where the reader may complete it.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];
type ContactMoment = components["schemas"]["ContactMoment"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-10T09:00:00Z",
  updated_at: "2026-08-10T09:00:00Z",
} as const;

function task(
  id: string,
  subject: string,
  due: string,
  writable = true,
): Activity {
  return {
    id,
    kind: "task",
    subject,
    occurred_at: "2026-08-10T09:00:00Z",
    occurred_precision: "instant",
    offers_email: false,
    due_at: due,
    is_done: false,
    writable,
    version: 3,
    ...CAPTURED,
  };
}

const TASKS = [
  task("t-1", "Send the price sheet", "2026-08-14T09:00:00Z"),
  task("t-2", "Book the site visit", "2026-08-20T09:00:00Z"),
  task("t-3", "Confirm the delivery date", "2026-09-02T09:00:00Z"),
];

// A move the bar shows in place of a task: our message waits for their reply.
const WAITING: ContactMoment = {
  claim_key: "moment:awaiting_reply",
  evidence_fingerprint: "fp",
  rule: "awaiting_reply",
  headline: "Waiting for their reply",
  why_now: "Your last message went out 2 days ago.",
  confidence: "observed_fact",
  evidence: [{ type: "activity", id: "a-sent", label: "Our quote" }],
  recommended_action: {
    kind: "draft_reply",
    label: "Follow up",
    state: "available",
  },
};

function view(
  tasks: Activity[],
  moment?: ContactMoment,
  omitted: Contact360["sections_omitted"] = [],
): Contact360 {
  return {
    as_of: "2026-08-13T09:00:00Z",
    contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
    sections_omitted: omitted,
    next_steps: { data: tasks, page: { has_more: false } },
    moment,
  };
}

function Card() {
  const read = useContact360("p-1");
  if (!read.data) return null;
  return (
    <>
      <p>{read.data.contact.full_name}</p>
      <ContactOpenTasks view={read.data} onAllTasks={() => {}} />
    </>
  );
}

function mount(page: () => Contact360, routes: RouteMap = {}) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create", "update"] }),
    "GET /contacts/p-1/360": () => jsonResponse(page()),
    ...routes,
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ViewerZoneProvider zone={FIXTURE_TIMEZONE}>
          <ToastProvider>
            <Card />
            <ToastRegion />
          </ToastProvider>
        </ViewerZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function listed(): string[] {
  return TASKS.map((row) => row.subject ?? "").filter(
    (subject) => screen.queryByText(subject) !== null,
  );
}

it("lists every open task, soonest first, while the bar shows a move", async () => {
  mount(() => view(TASKS, WAITING));

  await screen.findByText("Send the price sheet");
  expect(listed()).toEqual([
    "Send the price sheet",
    "Book the site visit",
    "Confirm the delivery date",
  ]);
});

it("leaves out the task the Next bar already shows", async () => {
  mount(() => view(TASKS));

  await screen.findByText("Book the site visit");
  expect(listed()).toEqual([
    "Book the site visit",
    "Confirm the delivery date",
  ]);
});

it("says a task's due day once, beside who holds it", async () => {
  mount(() => view(TASKS, WAITING));

  await screen.findByText("Confirm the delivery date");
  expect(screen.getAllByText(/2 Sept?/)).toHaveLength(1);
});

it("completes a task on the tick, and the row leaves on the re-read", async () => {
  let done = false;
  const ifMatch: (string | null)[] = [];
  mount(() => view(done ? TASKS.slice(1) : TASKS, WAITING), {
    "PATCH /activities/t-1": () => {
      done = true;
      return jsonResponse({ ...TASKS[0], is_done: true, version: 4 });
    },
  });
  const routed = globalThis.fetch;
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    if ((request?.method ?? init?.method) === "PATCH") {
      ifMatch.push(request ? request.headers.get("If-Match") : null);
    }
    return routed(input, init);
  });
  const user = userEvent.setup();

  await screen.findByText("Send the price sheet");
  await user.click(
    screen.getByRole("checkbox", { name: "Mark Send the price sheet done" }),
  );

  await waitFor(() =>
    expect(screen.queryByText("Send the price sheet")).toBeNull(),
  );
  expect(ifMatch).toEqual(["3"]);
});

it("draws no card when no task is open", async () => {
  mount(() => view([]));

  await screen.findByText("Dana Buyer");
  expect(screen.queryByText(en["contact.card.openTasks"])).toBeNull();
});

it("draws no card when the Next bar holds the only open task", async () => {
  mount(() => view(TASKS.slice(0, 1)));

  await screen.findByText("Dana Buyer");
  expect(screen.queryByText(en["contact.card.openTasks"])).toBeNull();
});

it("draws no tick on a task the reader may not complete", async () => {
  mount(() =>
    view(
      [task("t-9", "Colleague's call", "2026-08-15T09:00:00Z", false)],
      WAITING,
    ),
  );

  await screen.findByText("Colleague's call");
  expect(screen.queryByRole("checkbox")).toBeNull();
});

it("says the tasks are hidden when the section is withheld", async () => {
  mount(() => view([], WAITING, ["next_steps"]));

  await screen.findByText(en["state.withheld"]);
  expect(screen.getByText(en["contact.card.openTasks"])).toBeTruthy();
});
