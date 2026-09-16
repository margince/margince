/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ContactBriefCard } from "./contactbrief";
import { ContactMemory } from "./contactmemory";
import { ContactRail } from "./contactrail";
import { installFetchStub, jsonResponse, meRoute } from "./story-utils";

// What the contact page says about the transport that carried a message.
//
// Since ADR-0107/A158 the interaction KIND and the transport that carried it
// are separate axes, and every surface here used to read the second off the
// first. On a contact captured over a chat channel — no address, no number —
// that produced a page that claimed mail three times over: an envelope on the
// timeline, "Email thread" under the brief, and a consent block reporting the
// contact as reachable by email while showing no row at all for the channel the
// CRM can actually reach them on.
//
// Every assertion below is made in BOTH directions. A page that stopped drawing
// envelopes by drawing nothing would pass a one-directional suite, and a mail
// contact still owes the reader an envelope.

type Contact360 = components["schemas"]["Contact360"];
type ContactBrief = components["schemas"]["ContactBrief"];
type Activity = components["schemas"]["Activity"];

const AT = "2026-08-15T09:00:00Z";

function anActivity(over: Partial<Activity>): Activity {
  return {
    id: "a-1",
    kind: "email",
    direction: "inbound",
    subject: "Re: the pilot",
    body: "they wrote",
    occurred_at: AT,
    created_at: AT,
    updated_at: AT,
    is_done: false,
    source: "gmail",
    version: 1,
    ...over,
  } as Activity;
}

// A chat message, carried by a provider the directory below knows a name for.
const aChatMessage = anActivity({
  id: "a-chat",
  kind: "message",
  channel_provider: "zalo_oa",
  subject: null,
  body: "họ đã nhắn tin",
  source: "ext:zalo-oa:zalo",
});

// A mail the reader may read, carrying the summary the server sets for every
// kind=email row. Without it this row models a shape the API does not send,
// and the citation would be judged against a message that is not one.
const aReadableMail = anActivity({
  id: "a-mail",
  kind: "email",
  subject: "Re: the pilot",
  email_summary: {
    activity_id: "a-mail",
    occurred_at: AT,
    display_status: "team",
    attachment_count: 0,
    move: "none",
    version: 1,
    subject: "Re: the pilot",
    preview: "they wrote",
  },
});

// A withheld mail that STILL carries its subject, its body and a summary. The
// server strips all three, so this row cannot come off the wire — which is the
// point: the card's own check is what stands between a response assembled by a
// path that forgot and a reader seeing mail that is not theirs.
const aWithheldMail = anActivity({
  id: "a-withheld",
  kind: "email",
  subject: "Angebot Q4",
  body: "Können wir Dienstag sprechen?",
  content_state: "withheld",
  email_summary: {
    activity_id: "a-withheld",
    occurred_at: AT,
    display_status: "withheld",
    attachment_count: 0,
    move: "none",
    version: 1,
    subject: "Angebot Q4",
    preview: "Können wir Dienstag sprechen?",
  },
});

// Everything the three surfaces read, and nothing inherited from a fixture that
// might change for another test's reasons.
function viewWith(
  options: Readonly<{
    emails?: boolean;
    phones?: boolean;
    reachability?: { provider: string; reachable: boolean }[];
    activities?: Activity[];
  }>,
): Contact360 {
  return {
    as_of: AT,
    contact: {
      id: "p-1",
      full_name: "Dana Buyer",
      source: "manual",
      captured_by: "human:u-1",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-06-01T08:00:00Z",
      version: 1,
      emails: options.emails
        ? [
            {
              id: "pe-1",
              email: "dana@brandt.example",
              is_primary: true,
              email_type: "work",
              position: 0,
              source: "manual",
              captured_by: "human:u-1",
            },
          ]
        : [],
      phones: options.phones
        ? [
            {
              id: "pp-1",
              phone: "+49 30 111",
              phone_type: "work",
              is_primary: true,
              position: 0,
              source: "manual",
              captured_by: "human:u-1",
            },
          ]
        : [],
      reachability: (options.reachability ?? []).map((entry) => ({
        ...entry,
        since: AT,
      })),
    },
    activities: {
      data: options.activities ?? [],
      page: { has_more: false },
    },
    sections_omitted: [],
  };
}

// A brief whose one sentence cites one record — the shape the chip row reads.
function briefCiting(
  entityType: ContactBrief["sentences"][number]["evidence"][number]["entity_type"],
  entityId: string,
): ContactBrief {
  return {
    contact_id: "p-1",
    generated_at: AT,
    generated_by: "deterministic",
    sentences: [
      {
        text: "They asked about the pilot.",
        evidence: [{ entity_type: entityType, entity_id: entityId }],
      },
    ],
  };
}

function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// The transport directory, which is what names a provider for a human. It is a
// deployment fact, so the label can only come from here — a switch in a screen
// would be right for whatever this build happens to ship and wrong for the next
// unit somebody drops under extensions/.
beforeEach(() => {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /channel-providers": () =>
      jsonResponse({
        data: [
          {
            provider: "zalo_oa",
            label: "Zalo OA",
            credential_model: "workspace_bot",
            supplies_transport: true,
          },
        ],
      }),
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// The timeline row's transport, which no longer sits in a badge of its own.
// The avatar-row redesign folded it into the row's meta line as plain text,
// right after who the row is with: read by that position rather than by the
// text, so the assertion is about what the meta line SAYS and not a query
// that had to know the answer first. The sibling selector, not a child one,
// because a mail row's "Unanswered" status is a `.badge` in that same line
// too, and a child selector could not tell the two spans apart.
function timelineTransportCell(container: HTMLElement): HTMLElement {
  const cell = container.querySelector<HTMLElement>(".pe-memory-who + span");
  if (!cell) {
    throw new Error("no transport text rendered on the timeline row");
  }
  return cell;
}

// The brief chip's transport, drawn as an icon beside its own words rather
// than in a badge: the chip row's own shape, `.pe-source`, one per cited
// record. Read by shape rather than by text for the same reason as the
// timeline: the assertion is about what the chip SAYS.
function chipTransportCell(container: HTMLElement): HTMLElement {
  const cell = container.querySelector<HTMLElement>(".pe-source");
  if (!cell) {
    throw new Error("no source chip rendered");
  }
  return cell;
}

describe("the conversation timeline", () => {
  it("draws a chat message as a message, named by its transport", async () => {
    const { container } = render(
      <ContactMemory view={viewWith({ activities: [aChatMessage] })} />,
    );
    // The name arrives with the transport directory, which is a fetch.
    await screen.findAllByText("Zalo OA");

    const cell = timelineTransportCell(container);
    expect(cell.textContent).toBe("Zalo OA");
    // The kind's glyph beside the word, the rule every source on the page
    // follows, so a chat row is told from a mail row at a glance.
    expect(cell.querySelector(".lucide-message-square")).toBeTruthy();
    // The reported symptom: an envelope on a contact with no email address.
    expect(cell.querySelector(".lucide-mail")).toBeNull();
  });

  it("still draws mail as mail", () => {
    const { container } = render(
      <ContactMemory view={viewWith({ activities: [anActivity({})] })} />,
    );

    const cell = timelineTransportCell(container);
    expect(cell.textContent).toBe("Email");
    // Left red for the same reason as the chat case above: no kind glyph is
    // drawn on the timeline row for any kind, mail included.
    expect(cell.querySelector(".lucide-mail")).toBeTruthy();
    expect(cell.querySelector(".lucide-message-square")).toBeNull();
  });
});

describe("the relationship brief's source chips", () => {
  it("names the transport of a cited chat message", async () => {
    const { container } = render(
      <ContactBriefCard
        brief={briefCiting("activity", "a-chat")}
        loading={false}
        view={viewWith({ activities: [aChatMessage] })}
      />,
    );
    // The chip's day rides beside the name, so the reader can tell one chat
    // citation from another on the same conversation.
    await screen.findByText("Zalo OA 15 Aug");

    const chip = chipTransportCell(container);
    expect(chip.textContent).toBe("Zalo OA 15 Aug");
    expect(chip.querySelector(".lucide-message-square")).toBeTruthy();
    expect(chip.querySelector(".lucide-mail")).toBeNull();
  });

  // A cited EMAIL is named by its SUBJECT and opens, like every other citation
  // of a message in the product. Naming the transport told a reader which pipe
  // carried the sentence and nothing about which message — and the message is
  // what they are reaching for when they press a source.
  it("names a cited mail by its subject and opens it", async () => {
    const onOpenEmail = vi.fn();
    render(
      <ContactBriefCard
        brief={briefCiting("activity", "a-mail")}
        loading={false}
        view={viewWith({ activities: [aReadableMail] })}
        onOpenEmail={onOpenEmail}
      />,
    );

    const cite = screen.getByRole("button", { name: /Re: the pilot/ });
    await userEvent.click(cite);
    expect(onOpenEmail).toHaveBeenCalledWith("a-mail");
  });

  // The transport chip is still right for every kind that is NOT mail, which
  // is the direction a one-sided change would break: a card that stopped
  // naming transports altogether would pass an email-only assertion.
  it("still names the transport of a cited call", () => {
    const { container } = render(
      <ContactBriefCard
        brief={briefCiting("activity", "a-call")}
        loading={false}
        view={{
          ...viewWith({
            activities: [anActivity({ id: "a-call", kind: "call" })],
          }),
        }}
      />,
    );

    const chip = chipTransportCell(container);
    expect(chip.textContent).toBe("Call 15 Aug");
  });

  // A message this reader is outside the audience of names no subject and
  // opens nothing. The citation says a source exists without disclosing it.
  it("neither names nor opens a cited mail that is withheld", () => {
    const onOpenEmail = vi.fn();
    render(
      <ContactBriefCard
        brief={briefCiting("activity", "a-withheld")}
        loading={false}
        view={viewWith({ activities: [aWithheldMail] })}
        onOpenEmail={onOpenEmail}
      />,
    );

    expect(screen.queryByText("Angebot Q4")).toBeNull();
    expect(screen.getByText("Not shared with you")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Angebot Q4/ })).toBeNull();
    expect(onOpenEmail).not.toHaveBeenCalled();
  });

  // The brief cites what the WRITER read, which is not bounded by the page of
  // activities this reader was handed. A citation the page cannot resolve is
  // named for the one thing it certainly is.
  it("claims no transport for a citation it cannot resolve", () => {
    const { container } = render(
      <ContactBriefCard
        brief={briefCiting("activity", "a-not-on-this-page")}
        loading={false}
        view={viewWith({ activities: [aChatMessage] })}
      />,
    );

    const chip = chipTransportCell(container);
    expect(chip.textContent).toBe("Conversation");
    expect(chip.querySelector(".lucide-mail")).toBeNull();
    expect(chip.querySelector(".lucide-message-square")).toBeNull();
  });
});

describe("consent and channels", () => {
  function renderRail(view: Contact360) {
    render(
      <ContactRail
        view={view}
        guard={{
          contact_id: "p-1",
          entries: [
            {
              purpose_key: "business_correspondence",
              purpose_class: "business_correspondence",
              channel: "email",
              verdict: "allowed",
              reason: "she wrote to you on 15 Aug",
            },
          ],
        }}
      />,
    );
    return within(screen.getByTestId("contact-rail"));
  }

  it("asserts no verdict for a transport the record does not carry", () => {
    const rail = renderRail(
      viewWith({
        reachability: [{ provider: "zalo_oa", reachable: true }],
        activities: [aChatMessage],
      }),
    );

    // The reported symptom: "Email — Allowed" on a contact with no address.
    expect(rail.getByText("No address on file")).toBeTruthy();
    expect(rail.getByText("No number on file")).toBeTruthy();
  });

  it("gives the channel they are actually reached on its own row", async () => {
    const rail = renderRail(
      viewWith({
        reachability: [{ provider: "zalo_oa", reachable: true }],
        activities: [aChatMessage],
      }),
    );

    // Named by the directory and carrying the correspondence verdict, which the
    // send gate answers per purpose and not per transport.
    expect(await rail.findByText("Zalo OA")).toBeTruthy();
    expect(rail.getByText("Allowed")).toBeTruthy();
  });

  it("keeps the row when the identity can no longer be delivered to", async () => {
    const rail = renderRail(
      viewWith({
        reachability: [{ provider: "zalo_oa", reachable: false }],
        activities: [aChatMessage],
      }),
    );

    expect(await rail.findByText("Zalo OA")).toBeTruthy();
    expect(rail.getByText("Not deliverable")).toBeTruthy();
    // A blocked channel must not read as a granted one.
    expect(rail.queryByText("Allowed")).toBeNull();
  });

  it("still reports the verdict for a contact who has an address", () => {
    const rail = renderRail(
      viewWith({ emails: true, activities: [anActivity({})] }),
    );

    expect(rail.getByText("Allowed")).toBeTruthy();
    expect(rail.queryByText("No address on file")).toBeNull();
    expect(rail.getByText("she wrote to you on 15 Aug")).toBeTruthy();
  });
});

describe("a message the reader may not read", () => {
  it("says so, and says nothing else, whatever the row carries", () => {
    render(<ContactMemory view={viewWith({ activities: [aWithheldMail] })} />);

    // The row stays — a reader can tell a limited conversation from one that
    // never happened.
    expect(screen.getByText("Not shared with you")).toBeTruthy();
    // And none of what it carries reaches the card.
    expect(screen.queryByText("Angebot Q4")).toBeNull();
    expect(screen.queryByText(/Können wir Dienstag sprechen/)).toBeNull();
  });
});
