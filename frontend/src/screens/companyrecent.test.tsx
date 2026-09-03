/** @vitest-environment jsdom */

// What the account page's recent-exchange list draws for each kind it holds.
//
// The list shipped drawing `activity.subject` for every kind, email included,
// so a message here had no access badge, no counterparty, no preview and no
// way to open it — while the same message on the History tab had all four.
// Nothing failed, because this file did not exist.
//
// The claims are separable on purpose: an email is the canonical row, a
// withheld one discloses nothing, and every other kind keeps the kind chip and
// direction line that are the only things telling a call from a note.

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanyRecentList } from "./companyrecent";

type Activity = components["schemas"]["Activity"];
type EmailSummary = components["schemas"]["EmailSummary"];

afterEach(cleanup);

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

function renderList(
  activities: Activity[],
  onOpenRecord?: (entityType: string, entityId: string) => void,
) {
  return render(
    <LocaleProvider initial="en">
      <CompanyRecentList activities={activities} onOpenRecord={onOpenRecord} />
    </LocaleProvider>,
  );
}

describe("the account's recent exchanges", () => {
  it("draws a retained email as the canonical row, with its preview and not its body", () => {
    renderList([emailRow()]);

    expect(screen.getByText("Re: the renewal quote")).toBeTruthy();
    expect(
      screen.getByText("Can you hold the price until Friday?"),
    ).toBeTruthy();
    expect(screen.queryByText(WHOLE_BODY)).toBeNull();
    // Who may read it, which this list could not say at all before.
    expect(screen.getByText("Team")).toBeTruthy();
  });

  it("opens the message through the page's own router", async () => {
    const user = userEvent.setup();
    const onOpenRecord = vi.fn();
    renderList([emailRow()], onOpenRecord);

    await user.click(
      screen.getByRole("button", { name: /Re: the renewal quote/ }),
    );

    // "activity", not "email": the page's citation router already sends an
    // activity to the drawer, and a second vocabulary would be a second router.
    expect(onOpenRecord).toHaveBeenCalledWith("activity", EMAIL_ID);
  });

  it("discloses nothing from a withheld message", () => {
    renderList(
      [
        emailRow({
          content_state: "withheld",
          subject: null,
          body: null,
          email_summary: emailSummary({
            display_status: "withheld",
            subject: null,
            preview: null,
            counterparty: null,
          }),
        }),
      ],
      vi.fn(),
    );

    expect(screen.queryByText("Re: the renewal quote")).toBeNull();
    expect(screen.queryByText("Dana Buyer")).toBeNull();
    expect(screen.queryByText(WHOLE_BODY)).toBeNull();
    // The row still draws. A section that silently dropped what this reader
    // may not see would report a quieter account than the one on file.
    expect(screen.getByText("Withheld")).toBeTruthy();
  });

  it("leaves every other kind with its chip and its direction", () => {
    renderList([callRow], vi.fn());

    expect(screen.getByText("Depot walkthrough")).toBeTruthy();
    // The chip and the direction phrase are what tell a call from a note here,
    // and an email's own row states both in its own words instead.
    expect(screen.getByText("Call")).toBeTruthy();
    expect(screen.getByText("we called")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /Depot walkthrough/ }),
    ).toBeNull();
  });
});
