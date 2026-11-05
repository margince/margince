// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactActions } from "./contactactions";

type Contact360 = components["schemas"]["Contact360"];

// The contact header's action row: the verbs a rep reaches for daily in the
// open, everything else one press behind the ellipsis. The division is what
// the story is for — read the row closed first, then the menu.
const meta: Meta = {
  title: "Records/Contact 360/Header actions",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const CAPTURED = {
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-05-01T08:00:00Z",
  updated_at: "2026-07-01T08:00:00Z",
} as const;

// With an address and a number on file, so the lead verb is the mail one and
// Call dials. Each missing way to reach them is its own story below.
const VIEW: Contact360 = {
  as_of: "2026-08-13T09:00:00Z",
  contact: {
    id: "p-1",
    full_name: "Anna Weber",
    title: "Head of Procurement",
    writable: true,
    version: 3,
    primary_email: "anna.weber@brandt.example",
    emails: [
      {
        id: "e-1",
        contact_id: "p-1",
        email: "anna.weber@brandt.example",
        email_type: "work",
        is_primary: true,
        position: 0,
        ...CAPTURED,
      },
    ],
    phones: [
      {
        id: "ph-1",
        contact_id: "p-1",
        phone: "+49 30 1234 5678",
        phone_type: "work",
        is_primary: true,
        position: 0,
        ...CAPTURED,
      },
    ],
    ...CAPTURED,
  },
  sections_omitted: [],
  activities: { data: [], page: { has_more: false } },
  deal_roles: { data: [], page: { has_more: false } },
  profile_fields: [],
};

const ROUTES = {
  "GET /me": meRoute(
    { contact: ["read", "update"], activity: ["create"] },
    { seat: "full" },
  ),
  "GET /custom-fields": () => jsonResponse({ data: [] }),
  // Which messaging providers exist is a deployment fact, so a channel has no
  // name until the directory supplies one.
  "GET /channel-providers": () => jsonResponse({ data: [] }),
  // The reader's own connected mailbox, which is what makes an address on the
  // page the composer's rather than their mail client's.
  "GET /connectors": () =>
    jsonResponse({
      data: [{ id: "g1", provider: "gmail", status: "connected", scopes: [] }],
    }),
};

function Header({
  view = VIEW,
  refusedReasonId,
}: Readonly<{ view?: Contact360; refusedReasonId?: string }>) {
  return (
    // The head's own bar class, so the verbs share the width as tiles the way
    // they do on the record page.
    <div style={{ maxWidth: "18.75rem" }}>
      <div className="record-split-verbs">
        <ContactActions
          view={view}
          contactId={view.contact.id}
          onWrite={() => undefined}
          onResearch={() => undefined}
          onLogActivity={() => undefined}
          onAddTask={() => undefined}
          refusedReasonId={refusedReasonId}
        />
      </div>
    </div>
  );
}

export const Closed: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <Header />
      </StoryProviders>
    );
  },
};

// Every secondary verb, in order, worded and with no glyph among them —
// Archive last because it is the destructive one.
export const MenuOpen: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <Header />
      </StoryProviders>
    );
  },
  play: async () => {
    // The panel portals to the body, so it is reached through `screen` and
    // never through a canvas-scoped query.
    await userEvent.click(screen.getByRole("button", { name: "More actions" }));
    await screen.findByTestId("archive-record");
  },
};

// A contact this reader may not change: the rows stay and each points at the
// page's one sentence about why, which is the half a dimmed control cannot
// say. The activity verbs are unaffected — logging and mail carry gates of
// their own.
export const RefusedWrites: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <Header
          view={{ ...VIEW, contact: { ...VIEW.contact, writable: false } }}
          refusedReasonId="contact-not-writable"
        />
        <p id="contact-not-writable" className="t-caption">
          You cannot change this contact. Ask their owner to share them with
          you.
        </p>
      </StoryProviders>
    );
  },
  play: async () => {
    await userEvent.click(screen.getByRole("button", { name: "More actions" }));
    await screen.findByTestId("archive-record");
  },
};

// Nowhere to write to: the lead verb explains itself rather than merely
// dimming, and the rest of the row is untouched.
export const NoTransport: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <Header
          view={{
            ...VIEW,
            contact: { ...VIEW.contact, emails: [], primary_email: null },
          }}
        />
      </StoryProviders>
    );
  },
};

// No number on file: Call stays in the row and says why it cannot dial.
export const NoPhone: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <Header view={{ ...VIEW, contact: { ...VIEW.contact, phones: [] } }} />
      </StoryProviders>
    );
  },
};

// The second number is the primary one, so Call dials it.
export const SecondPhonePrimary: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <Header
          view={{
            ...VIEW,
            contact: {
              ...VIEW.contact,
              phones: [
                {
                  id: "ph-0",
                  contact_id: "p-1",
                  phone: "+49 30 9999 0000",
                  phone_type: "work",
                  is_primary: false,
                  position: 0,
                  ...CAPTURED,
                },
                {
                  id: "ph-1",
                  contact_id: "p-1",
                  phone: "+49 170 333 4444",
                  phone_type: "mobile",
                  is_primary: true,
                  position: 1,
                  ...CAPTURED,
                },
              ],
            },
          }}
        />
      </StoryProviders>
    );
  },
  play: async () => {
    const call = await screen.findByRole("link", { name: "Call" });
    await expect(call.getAttribute("href")).toBe("tel:+491703334444");
  },
};

// A role that may not log activity: the two log verbs are refused and the one
// sentence saying why hangs under the row as a caption, at the verbs' width,
// rather than setting the row's width and pushing the header's name aside.
export const LogRefused: Story = {
  render: () => {
    installFetchStub({
      ...ROUTES,
      "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
    });
    return (
      <StoryProviders>
        <Header />
      </StoryProviders>
    );
  },
};

// An archived contact takes no activity either: every verb points at the
// page's one sentence, and no second refusal is printed beside it.
export const Archived: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <Header
          view={{
            ...VIEW,
            contact: {
              ...VIEW.contact,
              writable: false,
              archived_at: "2026-08-14T09:00:00Z",
            },
          }}
          refusedReasonId="contact-archived"
        />
        <p id="contact-archived" className="t-caption">
          {en["contact.rail.archivedReadOnly"]}
        </p>
      </StoryProviders>
    );
  },
};
