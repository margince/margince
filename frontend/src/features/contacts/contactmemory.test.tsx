/** @vitest-environment happy-dom */

// What the What was said card draws: the latest two conversations. Each is a
// title over "{kind} · {who spoke} · {date}" with its reply state at the right.
// The History tab keeps the whole ledger, its faces and its Reply.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { telUri } from "../../format/contacturi";
import { LocaleProvider } from "../../i18n";
import { ContactMemory } from "./contactmemory";

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];
type EmailSummary = components["schemas"]["EmailSummary"];
type Entry = components["schemas"]["ConversationMemoryEntry"];

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const EMAIL_ID = "01a05500-0000-7000-8000-00000000ee01";
const NOTE_ID = "01a05500-0000-7000-8000-00000000ee02";

const contact: components["schemas"]["Contact"] = {
  id: "p-1",
  full_name: "Dana Buyer",
  first_name: "Dana",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  emails: [],
};

// The summary's preview and the activity's whole body differ. The card draws
// neither, and a fixture where they matched could not tell.
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
    counterparty_contact_id: "p-1",
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
    voice: {
      author: {
        kind: "contact_record",
        display_name: "Dana Buyer",
        contact_id: "p-1",
        is_reader: false,
      },
      contact_role: "author",
    },
    ...over,
  };
}

// P wrote to the reader with this page's contact in Cc. The server names P as
// the author, marks this contact as copied, and owes them no reply.
const copiedOnly: Partial<Activity> = {
  voice: {
    author: {
      kind: "contact_record",
      display_name: "Pia Sender",
      contact_id: "p-2",
      is_reader: false,
    },
    contact_role: "cc",
  },
  email_summary: emailSummary({ move: "none", counterparty: "Pia Sender" }),
};

const noteRow: Activity = {
  offers_email: false,
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

// Every instant fixed: `make fe-clock-drift` runs the suite at +200 days, and
// a row dated relative to today would read as a meeting still to come.
function notes(count: number): Activity[] {
  return Array.from({ length: count }, (_, index) => ({
    ...noteRow,
    id: `01a05500-0000-7000-8000-0000000000${10 + index}`,
    subject: `Note ${index + 1}`,
    occurred_at: `2026-08-${20 - index}T11:00:00Z`,
  }));
}

const onePage = { has_more: false, next_cursor: null };

function viewWith(
  activities: Activity[],
  over: Partial<Contact360> = {},
): Contact360 {
  return {
    as_of: "2026-08-30T09:00:00Z",
    contact,
    sections_omitted: [],
    activities: { data: activities, page: onePage },
    ...over,
  };
}

function renderCard(
  view: Contact360,
  onOpenEmail: (activityId: string) => void = vi.fn(),
  onFullHistory: () => void = vi.fn(),
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactMemory
          view={view}
          onOpenEmail={onOpenEmail}
          onFullHistory={onFullHistory}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the What was said card", () => {
  it("draws an email as its subject over kind, side and day, never its words", () => {
    renderCard(viewWith([emailRow()]));

    expect(screen.getByText("Re: the renewal quote")).toBeTruthy();
    expect(screen.getByText("Email · they wrote · 29 Aug")).toBeTruthy();
    expect(screen.getByText("Unanswered")).toBeTruthy();
    expect(screen.queryByText(WHOLE_BODY)).toBeNull();
    expect(
      screen.queryByText("Can you hold the price until Friday?"),
    ).toBeNull();
  });

  it("names the real author of a mail this contact was only copied on", () => {
    renderCard(viewWith([emailRow(copiedOnly)]));

    expect(
      screen.getByText("Email · Pia Sender wrote · in Cc · 29 Aug"),
    ).toBeTruthy();
    expect(screen.queryByText(/they wrote/)).toBeNull();
    expect(screen.queryByText("Unanswered")).toBeNull();
  });

  it("names a colleague who wrote, never the reader", () => {
    renderCard(
      viewWith([
        emailRow({
          direction: "outbound",
          voice: {
            author: {
              kind: "colleague",
              display_name: "Carla",
              is_reader: false,
            },
            contact_role: "addressee",
          },
          email_summary: emailSummary({ move: "waiting_for_them" }),
        }),
      ]),
    );

    expect(screen.getByText("Email · Carla wrote · 29 Aug")).toBeTruthy();
  });

  it("names no author on a mail whose author nothing recorded", () => {
    renderCard(viewWith([emailRow({ voice: undefined })]));

    expect(screen.getByText("Email · 29 Aug")).toBeTruthy();
  });

  it("owes no reply on a chat message this contact was only copied on", () => {
    renderCard(
      viewWith([
        emailRow({
          kind: "message",
          channel_provider: "zalo_oa",
          email_summary: null,
          voice: copiedOnly.voice,
        }),
      ]),
    );

    expect(screen.queryByText("Unanswered")).toBeNull();
  });

  it("opens the page's drawer on the email the row is about", async () => {
    const user = userEvent.setup();
    const onOpenEmail = vi.fn();
    renderCard(viewWith([emailRow()]), onOpenEmail);

    await user.click(
      screen.getByRole("button", { name: "Re: the renewal quote" }),
    );

    expect(onOpenEmail).toHaveBeenCalledWith(EMAIL_ID);
  });

  it("draws a note as plain words, with no side and nothing to open", () => {
    renderCard(viewWith([noteRow]));

    expect(screen.getByText("Call prep")).toBeTruthy();
    expect(screen.getByText("Note · 28 Aug")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Call prep" })).toBeNull();
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
    );

    expect(screen.queryByText("Re: the renewal quote")).toBeNull();
    expect(screen.queryByText(WHOLE_BODY)).toBeNull();
    // The row stays, saying it is not shared: drawn as absent, a private
    // conversation would read as one that never was.
    expect(screen.getByText("Not shared with you")).toBeTruthy();
  });

  it("draws the latest two and leaves the rest to Full history", async () => {
    const user = userEvent.setup();
    const onFullHistory = vi.fn();
    renderCard(viewWith(notes(4)), vi.fn(), onFullHistory);

    expect(screen.getByText("Note 1")).toBeTruthy();
    expect(screen.getByText("Note 2")).toBeTruthy();
    expect(screen.queryByText("Note 3")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Full history" }));
    expect(onFullHistory).toHaveBeenCalledOnce();
  });

  it("reads the thread projection when the server has filled it", () => {
    const entry: Entry = {
      key: "thread-1",
      channel: "meeting",
      direction: "internal",
      title: "Site visit",
      summary: "Walked the depot.",
      occurred_at: "2026-08-20T09:00:00Z",
      activity_count: 1,
      status: "awaiting_them",
    };
    renderCard(viewWith([emailRow()], { conversation_memory: [entry] }));

    expect(screen.getByText("Site visit")).toBeTruthy();
    expect(screen.getByText("Meeting · 20 Aug")).toBeTruthy();
    expect(screen.queryByText(/both sides/)).toBeNull();
    // An entry names no author or page role, so it claims no duty either.
    expect(screen.queryByText("Waiting on them")).toBeNull();
    expect(screen.queryByText("Re: the renewal quote")).toBeNull();
  });

  it("names nobody and owes nothing on a thread entry with no author", () => {
    const entry: Entry = {
      key: "thread-2",
      channel: "email",
      direction: "inbound",
      title: "Signed NDA please",
      summary: "",
      occurred_at: "2026-08-20T09:00:00Z",
      activity_count: 1,
      status: "unanswered",
    };
    renderCard(viewWith([emailRow()], { conversation_memory: [entry] }));

    expect(screen.getByText("Email · 20 Aug")).toBeTruthy();
    expect(screen.queryByText("Unanswered")).toBeNull();
  });

  it.each(["call", "meeting", "message"] as const)(
    "keeps an unvoiced %s entry neutral across server versions",
    (channel) => {
      const entry: Entry = {
        key: "legacy-conversation",
        channel,
        direction: "outbound",
        title: "Pricing discussion",
        summary: "",
        occurred_at: "2026-08-20T09:00:00Z",
        activity_count: 1,
        status: "awaiting_them",
      };
      renderCard(viewWith([], { conversation_memory: [entry] }));

      expect(screen.getByText("Pricing discussion")).toBeTruthy();
      expect(
        screen.queryByText(
          /they wrote|you wrote|they called|you called|both sides/,
        ),
      ).toBeNull();
      expect(screen.queryByText("Waiting on them")).toBeNull();
    },
  );

  it("does not say who dialled a logged call", () => {
    renderCard(
      viewWith([
        emailRow({
          kind: "call",
          email_summary: null,
          subject: "Pricing call",
        }),
      ]),
    );

    expect(screen.queryByText(/they called|you called/)).toBeNull();
  });

  it("says nothing was said rather than drawing an empty card", () => {
    renderCard(viewWith([]));
    expect(screen.getByText("No conversation captured yet.")).toBeTruthy();
  });

  it("says the conversations are hidden when both reads are withheld", () => {
    renderCard(
      viewWith([], {
        activities: undefined,
        sections_omitted: ["activities", "conversation_memory"],
      }),
    );
    expect(screen.getByText("Hidden for your role")).toBeTruthy();
  });
});

it("groups replies by conversation without clearing another request from the same contact", () => {
  const reply = emailRow({
    id: "reply",
    thread_key: "answered",
    direction: "outbound",
    occurred_at: "2026-08-30T09:00:00Z",
    email_summary: emailSummary({
      activity_id: "reply",
      subject: "Competitor question",
      direction: "outbound",
      move: "none",
    }),
  });
  const question = emailRow({
    id: "question",
    thread_key: "answered",
    email_summary: emailSummary({
      activity_id: "question",
      subject: "Competitor question",
      move: "none",
    }),
  });
  const request = emailRow({
    thread_key: "report",
    email_summary: emailSummary({
      subject: "Please send the report",
      move: "needs_reply",
    }),
  });
  renderCard(viewWith([reply, question, request]));
  expect(screen.getAllByText("Competitor question")).toHaveLength(1);
  expect(screen.getByText("Please send the report")).toBeTruthy();
  expect(screen.getByText("Unanswered")).toBeTruthy();
});

type ContactPhone = components["schemas"]["ContactPhone"];

const OFFICE = "+49 30 1234567";
const MOBILE = "+49 151 7654321";

function phone(id: string, number: string, isPrimary: boolean): ContactPhone {
  return {
    id,
    phone: number,
    phone_type: "work",
    is_primary: isPrimary,
    position: 0,
    source: "manual",
    captured_by: "human:u-1",
  };
}

// The primary number comes second, so a card that dialled the first number
// listed would fail here.
const reachable = {
  ...contact,
  phones: [phone("ph-0", OFFICE, false), phone("ph-1", MOBILE, true)],
};

const callRow: Activity = {
  ...noteRow,
  id: "01a05500-0000-7000-8000-00000000ee03",
  kind: "call",
  subject: "Called about the renewal",
  direction: "outbound",
};

const callEntry: Entry = {
  key: "thread-call",
  channel: "call",
  direction: "outbound",
  title: "Called about the renewal",
  summary: "Walked through the quote.",
  occurred_at: "2026-08-28T11:00:00Z",
  activity_count: 1,
};

describe("Call on a logged call", () => {
  it("dials the primary number from a thread projection call", () => {
    renderCard(
      viewWith([], { contact: reachable, conversation_memory: [callEntry] }),
    );

    expect(
      screen.getByRole("link", { name: "Call" }).getAttribute("href"),
    ).toBe(telUri(MOBILE));
  });

  it("dials the primary number from a timeline call", () => {
    renderCard(viewWith([callRow], { contact: reachable }));

    expect(
      screen.getByRole("link", { name: "Call" }).getAttribute("href"),
    ).toBe(telUri(MOBILE));
  });

  it("offers no Call on a note", () => {
    renderCard(viewWith([noteRow], { contact: reachable }));

    expect(screen.getByText("Call prep")).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Call" })).toBeNull();
  });

  it("offers no Call when the contact has no phone", () => {
    renderCard(viewWith([callRow]));

    expect(screen.getByText("Called about the renewal")).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Call" })).toBeNull();
  });

  it("offers no Call on an archived contact", () => {
    renderCard(
      viewWith([callRow], {
        contact: { ...reachable, archived_at: "2026-08-29T09:00:00Z" },
      }),
    );

    expect(screen.getByText("Called about the renewal")).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Call" })).toBeNull();
  });
});
