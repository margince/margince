// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// A notice about a mail or a meeting opens that activity from its name, on the
// Home page's Updates panel where such notices are read.

import "@testing-library/jest-dom/vitest";
import {
  focusManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { BriefUpdates } from "../brief/brief.feed";
import { day, jsonResponse, row, type WorklistItem } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const MAIL_ID = "01a05500-0000-7000-8000-0000000000f1";
const MEETING_ID = "01a05500-0000-7000-8000-0000000000f2";

function notice(over: Partial<WorklistItem>): WorklistItem {
  return row({
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "notice",
    category: "tasks",
    level: 3,
    title: "Summary updated",
    actions: ["acknowledge"],
    ...over,
  });
}

const mailNotice = notice({
  subject: { type: "activity", id: MAIL_ID, label: "Pricing for Q4" },
});

const mail = {
  id: MAIL_ID,
  kind: "email",
  subject: "Pricing for Q4",
  occurred_at: "2026-09-01T09:00:00Z",
  source: "capture",
  captured_by: "human:u-1",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  content_state: "available",
  links: [],
};

const presentation = {
  id: MAIL_ID,
  lifecycle: "delivered",
  occurred_at: "2026-09-01T09:00:00Z",
  summary: {
    activity_id: MAIL_ID,
    occurred_at: "2026-09-01T09:00:00Z",
    version: 1,
    subject: "Pricing for Q4",
    preview: "Here is the price list.",
    counterparty: "Dana Buyer",
    direction: "inbound",
    display_status: "team",
    attachment_count: 0,
  },
  body: "Here is the price list.",
  thread_key: "t1",
  from: [{ address: "dana@acme.example", display_name: "Dana Buyer" }],
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
  can_reply: false,
  can_relink: false,
  version: 1,
};

const meeting = {
  ...mail,
  id: MEETING_ID,
  kind: "meeting",
  subject: "Quarterly review",
  body: "We agreed on the rollout.",
};

function stubServer() {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url.includes(`/activities/${MAIL_ID}/email-presentation`)) {
      return jsonResponse(presentation);
    }
    if (url.includes(`/activities/${MAIL_ID}`)) return jsonResponse(mail);
    if (url.includes(`/activities/${MEETING_ID}`)) return jsonResponse(meeting);
    return jsonResponse({ data: [] });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

// How many requests named this path, query aside.
function readsOf(fetchMock: ReturnType<typeof stubServer>, path: string) {
  return fetchMock.mock.calls.filter((call) => {
    const url = call[0] instanceof Request ? call[0].url : String(call[0]);
    return new URL(url, "http://localhost").pathname.endsWith(path);
  }).length;
}

function renderUpdates(queue: WorklistItem[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <BriefUpdates day={day({ queue })} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a notice about an activity", () => {
  it("opens the mail it names in the email drawer", async () => {
    const fetchMock = stubServer();
    renderUpdates([mailNotice]);

    const name = screen.getByRole("button", {
      name: "Summary updated · Pricing for Q4",
    });
    expect(name).toHaveAttribute("aria-haspopup", "dialog");
    await userEvent.setup().click(name);

    const drawer = await screen.findByRole("dialog");
    expect(
      await within(drawer).findByRole("heading", { name: "Pricing for Q4" }),
    ).toBeVisible();
    expect(
      readsOf(fetchMock, `/activities/${MAIL_ID}/email-presentation`),
    ).toBe(1);
    // The kind read is the drawer's only read of the activity itself.
    expect(readsOf(fetchMock, `/activities/${MAIL_ID}`)).toBe(1);
  });

  it("opens a meeting it names in the activity drawer", async () => {
    const fetchMock = stubServer();
    renderUpdates([
      notice({
        subject: {
          type: "activity",
          id: MEETING_ID,
          label: "Quarterly review",
        },
      }),
    ]);

    await userEvent.setup().click(
      screen.getByRole("button", {
        name: "Summary updated · Quarterly review",
      }),
    );

    const drawer = await screen.findByRole("dialog");
    expect(
      await within(drawer).findByText("We agreed on the rollout."),
    ).toBeVisible();
    expect(readsOf(fetchMock, `/activities/${MEETING_ID}`)).toBe(1);
  });

  it("reads nothing more once its drawer is closed", async () => {
    const fetchMock = stubServer();
    const user = userEvent.setup();
    renderUpdates([
      notice({
        subject: {
          type: "activity",
          id: MEETING_ID,
          label: "Quarterly review",
        },
      }),
    ]);
    await user.click(
      screen.getByRole("button", {
        name: "Summary updated · Quarterly review",
      }),
    );
    await screen.findByText("We agreed on the rollout.");

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    // A reader coming back to the tab is what refetches a live read.
    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });

    expect(readsOf(fetchMock, `/activities/${MEETING_ID}`)).toBe(1);
  });

  // The server leaves the label off an activity this reader may not read, mail
  // outside their audience included.
  it("offers nothing to open when the reader may not read the activity", () => {
    const fetchMock = stubServer();
    renderUpdates([notice({ subject: { type: "activity", id: MAIL_ID } })]);

    expect(screen.getByText("Summary updated")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "Summary updated" }),
    ).toBeNull();
    expect(readsOf(fetchMock, `/activities/${MAIL_ID}`)).toBe(0);
  });
});
