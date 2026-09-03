/** @vitest-environment jsdom */

// What the person page's memory card draws for each kind it holds.
//
// The card shipped reading `email_summary` for its title and preview while
// painting its own two lines with them, so a message read one way here and
// another way on the timeline six inches above it: no access badge, no mail
// icon, the whole body instead of a preview, and no way to open the message.
// Nothing failed, because this file did not exist.
//
// Three claims, and each fails on its own. An email is the canonical row; a
// withheld one says so and discloses nothing; every other kind keeps the two
// lines the card has always drawn for it.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { PersonMemory } from "./personmemory";

type Person360 = components["schemas"]["Person360"];
type Activity = components["schemas"]["Activity"];
type EmailSummary = components["schemas"]["EmailSummary"];

afterEach(cleanup);

const EMAIL_ID = "01a05500-0000-7000-8000-00000000ee01";
const NOTE_ID = "01a05500-0000-7000-8000-00000000ee02";

const person: components["schemas"]["Person"] = {
  id: "p-1",
  full_name: "Dana Buyer",
  first_name: "Dana",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  emails: [],
};

// A retained email, as the server sends one: the summary carries the subject
// and the preview, and the activity's own body carries the whole message. The
// two differ on purpose — a card drawing the body where the preview belongs is
// exactly the defect, and a fixture whose body equalled its preview could not
// tell the two apart.
const WHOLE_BODY =
  "Can you hold the price until Friday? " +
  "I have to take it past procurement first and they meet on Thursday.";

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

// A note on the same card. It has no email_summary and never had one, so it is
// the control for "only the email branch moved".
const noteRow: Activity = {
  id: NOTE_ID,
  kind: "note",
  occurred_at: "2026-08-28T11:00:00Z",
  subject: "Call prep",
  body: "Wants the fleet numbers before we talk pricing.",
  content_state: "available",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-28T11:00:00Z",
  updated_at: "2026-08-28T11:00:00Z",
  version: 1,
  is_done: false,
};

const onePage = { has_more: false, next_cursor: null };

// The activity rows above are typed, so a fixture that drifts from the wire
// shape fails to compile. The view around them is asserted rather than built:
// Person360 carries twenty-odd sections this card never reads, and spelling
// them all would describe the page instead of the card.
function viewWith(activities: Activity[]): Person360 {
  return {
    as_of: "2026-08-30T09:00:00Z",
    person,
    sections_omitted: [],
    activities: { data: activities, page: onePage },
  } as Person360;
}

// The card reads the transport directory for its channel labels, so it needs a
// client. Nothing here depends on that read resolving: every claim below is
// about an email, whose label comes from the kind.
function renderCard(
  view: Person360,
  onOpenEmail?: (activityId: string) => void,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <PersonMemory view={view} onOpenEmail={onOpenEmail} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the person page's memory card", () => {
  it("draws a retained email as the canonical row, with its preview and not its body", () => {
    renderCard(viewWith([emailRow()]));

    expect(screen.getByText("Re: the renewal quote")).toBeTruthy();
    expect(
      screen.getByText("Can you hold the price until Friday?"),
    ).toBeTruthy();
    // The whole message belongs to the drawer. Printing it here is what the
    // card did before, and it is what a reader saw instead of a row.
    expect(screen.queryByText(WHOLE_BODY)).toBeNull();
    // The access state travels with the message. Its absence was how a reader
    // could not tell a team-wide thread from a private one.
    expect(screen.getByText("Team")).toBeTruthy();
  });

  it("opens the page's drawer on the message the row is about", async () => {
    const user = userEvent.setup();
    const onOpenEmail = vi.fn();
    renderCard(viewWith([emailRow()]), onOpenEmail);

    await user.click(
      screen.getByRole("button", { name: /Re: the renewal quote/ }),
    );

    expect(onOpenEmail).toHaveBeenCalledWith(EMAIL_ID);
  });

  it("offers no opener when the page mounts no drawer", () => {
    renderCard(viewWith([emailRow()]));

    // Readable, and not pressable: a row that looks openable and answers
    // nothing teaches a reader the product is broken.
    expect(
      screen.queryByRole("button", { name: /Re: the renewal quote/ }),
    ).toBeNull();
    expect(screen.getByText("Re: the renewal quote")).toBeTruthy();
  });

  it("discloses nothing from a withheld message", () => {
    renderCard(
      viewWith([
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
      ]),
      vi.fn(),
    );

    expect(screen.queryByText("Re: the renewal quote")).toBeNull();
    expect(
      screen.queryByText("Can you hold the price until Friday?"),
    ).toBeNull();
    expect(screen.queryByText(WHOLE_BODY)).toBeNull();
    // The row stays, saying it is limited. Drawing it as absent would leave a
    // reader unable to tell a private conversation from one that never was.
    expect(screen.getByText("Withheld")).toBeTruthy();
  });

  it("leaves every other kind reading as it did", () => {
    renderCard(viewWith([noteRow]), vi.fn());

    expect(screen.getByText("Call prep")).toBeTruthy();
    // A note's own words, in full: the card's two lines are still the reading
    // for everything that is not an email.
    expect(
      screen.getByText("Wants the fleet numbers before we talk pricing."),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /Call prep/ }),
    ).toBeNull();
  });
});
