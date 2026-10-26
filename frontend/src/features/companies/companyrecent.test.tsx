/** @vitest-environment happy-dom */

// What the Overview's Latest card draws: the newest exchanges that already
// happened. Each is one line, with who, kind and day under the title, and
// "Unanswered" where an email waits on us.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { LatestCard } from "./companyoverviewlatest";

type Activity = components["schemas"]["Activity"];
type Company360 = components["schemas"]["Company360"];
type EmailSummary = components["schemas"]["EmailSummary"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const EMAIL_ID = "01a05500-0000-7000-8000-00000000cc01";
const CALL_ID = "01a05500-0000-7000-8000-00000000cc02";

// The subject and the preview differ from the body on purpose: a row drawing
// the body where the preview belongs is the defect, and a fixture whose three
// text fields agreed could not tell them apart.
const WHOLE_BODY =
  "Can you hold the price until Friday? Procurement meets on Thursday.";

function emailSummary(over: Partial<EmailSummary> = {}): EmailSummary {
  return {
    activity_id: EMAIL_ID,
    occurred_at: "2026-08-29T09:15:00Z",
    version: 4,
    subject: "Re: the renewal quote",
    preview: "Can you hold the price until Friday?",
    counterparty: "Dana Buyer",
    direction: "inbound",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 0,
    ...over,
  };
}

function emailRow(over: Partial<Activity> = {}): Activity {
  return {
    offers_email: false,
    id: EMAIL_ID,
    kind: "email",
    occurred_at: "2026-08-29T09:15:00Z",
    subject: "Re: the renewal quote",
    body: WHOLE_BODY,
    direction: "inbound",
    content_state: "available",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-08-29T09:15:00Z",
    updated_at: "2026-08-29T09:15:00Z",
    version: 4,
    is_done: false,
    email_summary: emailSummary(),
    ...over,
  };
}

// A call in the same list. It carries no summary and never will, so it is the
// control for "only the email branch moved".
const callRow: Activity = {
  offers_email: false,
  id: CALL_ID,
  kind: "call",
  occurred_at: "2026-08-28T14:00:00Z",
  subject: "Depot walkthrough",
  direction: "outbound",
  content_state: "available",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-28T14:00:00Z",
  updated_at: "2026-08-28T14:00:00Z",
  version: 1,
  is_done: false,
};

function view(activities: Activity[]): Company360 {
  return {
    as_of: "2026-09-01T09:00:00Z",
    company: {
      id: "o-1",
      display_name: "Brandt Automotive GmbH",
      source: "manual",
      captured_by: "human:u-1",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-06-01T08:00:00Z",
    },
    sections_omitted: [],
    activities: {
      data: activities,
      page: { has_more: false, next_cursor: null },
    },
  };
}

function renderCard(
  three60: Company360,
  onOpenEmail: (activityId: string) => void = () => {},
) {
  // The recorder's name is read from the roster; the stub answers it.
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({ id: "u-1", display_name: "Lena Fischer" }),
          { headers: { "content-type": "application/json" } },
        ),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <LatestCard
          view={three60}
          onOpenTab={() => {}}
          onOpenEmail={onOpenEmail}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the Overview's Latest card", () => {
  it("names the other side of an email and marks it unanswered", () => {
    renderCard(view([emailRow()]));
    const row = screen.getByText("Re: the renewal quote").closest("li");
    expect(row).not.toBeNull();
    const inRow = within(row as HTMLElement);
    expect(inRow.getByText("Dana Buyer · email · 29 Aug")).toBeTruthy();
    expect(inRow.getByText("Unanswered")).toBeTruthy();
    expect(screen.queryByText(WHOLE_BODY)).toBeNull();
  });

  it("opens the message through the host's email door", async () => {
    const opened = vi.fn();
    renderCard(view([emailRow()]), opened);
    await userEvent.click(
      screen.getByRole("button", { name: "Re: the renewal quote" }),
    );
    expect(opened).toHaveBeenCalledWith(EMAIL_ID);
  });

  it("lists the newest first and leaves out what has not happened yet", () => {
    const booked: Activity = {
      ...callRow,
      id: "01a05500-0000-7000-8000-00000000cc03",
      kind: "meeting",
      subject: "Retrofit walkthrough",
      occurred_at: "2026-09-20T10:00:00Z",
    };
    renderCard(view([callRow, emailRow(), booked]));
    const titles = screen
      .getAllByRole("listitem")
      .map((item) => item.textContent ?? "");
    expect(titles[0]).toContain("Re: the renewal quote");
    expect(titles[1]).toContain("Depot walkthrough");
    expect(screen.queryByText("Retrofit walkthrough")).toBeNull();
  });

  it("says there is nothing yet on an account with no exchange", () => {
    renderCard(view([]));
    expect(screen.getByText("No activity logged yet.")).toBeTruthy();
  });
});
