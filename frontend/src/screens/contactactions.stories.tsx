// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { components } from "../api/schema";
import { ContactActions } from "./contactactions";
import { useObjectCustomFields } from "./customfields.form";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

type Contact360 = components["schemas"]["Contact360"];

// The contact header's action row: the verbs a rep reaches for daily in the
// open, everything else one press behind the ellipsis. The division is what
// the story is for — read the row closed first, then the menu.
const meta: Meta = {
  title: "Records/Contact header actions",
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

// With an address on file, so the lead verb is the mail one — the row reads
// differently for a contact with nowhere to write to, and that refusal is its
// own story below.
const VIEW: Contact360 = {
  as_of: "2026-08-13T09:00:00Z",
  contact: {
    id: "p-1",
    full_name: "Anna Weber",
    title: "Head of Procurement",
    writable: true,
    version: 3,
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
  const cf = useObjectCustomFields("contact");
  return (
    // The header's own row class, borrowed so the verbs stand at the interval
    // the record page gives them rather than at whatever a bare story would.
    <div className="record-actions record-actions-inline">
      <ContactActions
        view={view}
        contactId={view.contact.id}
        cf={cf}
        overlay={false}
        onWrite={() => undefined}
        onResearch={() => undefined}
        onLogActivity={() => undefined}
        onAddTask={() => undefined}
        refusedReasonId={refusedReasonId}
      />
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
    await screen.findByTestId("edit-record");
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
    await screen.findByTestId("edit-record");
  },
};

// Nowhere to write to: the lead verb explains itself rather than merely
// dimming, and the rest of the row is untouched.
export const NoTransport: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <Header view={{ ...VIEW, contact: { ...VIEW.contact, emails: [] } }} />
      </StoryProviders>
    );
  },
};
