/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../testing/story-utils";
import { ContactConsentPanel } from "./contactconsentpanel";
import { ContactMemory } from "./contactmemory";
import { ContactRail } from "./contactrail";

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
    voice: {
      author: {
        kind: "contact_record",
        display_name: "Dana",
        is_reader: false,
      },
      contact_role: "author",
    },
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
  voice: undefined,
  email_summary: {
    activity_id: "a-withheld",
    occurred_at: AT,
    occurred_precision: "instant",
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

// The card names the transport in the row's line under the title, before who
// spoke. A chat provider shows the directory's name, mail the kind's word.
function memory(view: Contact360) {
  return render(
    <ContactMemory
      view={view}
      onOpenEmail={() => undefined}
      onFullHistory={() => undefined}
    />,
  );
}

describe("the What was said card", () => {
  it("names a chat message by its transport, never as mail", async () => {
    memory(viewWith({ activities: [aChatMessage] }));
    // The name arrives with the transport directory, which is a fetch.
    expect(
      await screen.findByText("Zalo OA · they wrote · 15 Aug"),
    ).toBeTruthy();
    expect(screen.queryByText(/^Email/)).toBeNull();
  });

  it("still names mail as mail", () => {
    memory(viewWith({ activities: [anActivity({})] }));
    expect(screen.getByText("Email · they wrote · 15 Aug")).toBeTruthy();
    expect(screen.queryByText(/Zalo OA/)).toBeNull();
  });
});

describe("consent and channels", () => {
  const GUARD: components["schemas"]["ContactConsentGuard"] = {
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
  };

  function renderRail(view: Contact360) {
    render(
      <ContactRail
        view={view}
        consent={{
          guard: GUARD,
          loading: false,
          failed: false,
          onRetry: () => undefined,
        }}
        writeRefusalId="write-refused"
      />,
    );
    return screen;
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
    // The column states the verdict; the reason behind it is the Research
    // tab's consent panel's to give.
    expect(rail.queryByText("she wrote to you on 15 Aug")).toBeNull();
  });

  it("gives each purpose's reason on the Research tab's consent panel", async () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /contacts/p-1/consent/guard": () => jsonResponse(GUARD),
    });
    render(
      <ContactConsentPanel
        view={viewWith({ emails: true, activities: [anActivity({})] })}
      />,
    );

    expect(await screen.findByText("she wrote to you on 15 Aug")).toBeTruthy();
    expect(screen.getByText("Allowed")).toBeTruthy();
  });
});

describe("a message the reader may not read", () => {
  it("says so, and says nothing else, whatever the row carries", () => {
    memory(viewWith({ activities: [aWithheldMail] }));

    // The row stays — a reader can tell a limited conversation from one that
    // never happened.
    expect(screen.getByText("Not shared with you")).toBeTruthy();
    // And none of what it carries reaches the card.
    expect(screen.queryByText("Angebot Q4")).toBeNull();
    expect(screen.queryByText(/Können wir Dienstag sprechen/)).toBeNull();
  });
});
