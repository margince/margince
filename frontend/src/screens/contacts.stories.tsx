// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { components } from "../api/schema";
import { ContactScreen, ContactsScreen } from "./contacts";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

type Contact = components["schemas"]["Contact"];

// ContactsScreen (list) and ContactScreen (360 Overview) both read through
// the api client on mount — fixtures mirror contacts.test.tsx's `anna` +
// dormant-strength default (the Overview tab fires the strength GET
// unconditionally).
const meta: Meta = {
  title: "Records/Contacts",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const anna = {
  id: "p-1",
  full_name: "Anna Weber",
  title: "Head of Procurement",
  emails: [{ id: "e-1", email: "anna.weber@brandt.example", is_primary: true }],
  captured_by: "connector:gmail",
  source: "gmail",
  version: 1,
};

const dormantStrength = {
  score: 0,
  bucket: "none",
  factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
  last_interaction: null,
};

export const ContactsList: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /contacts": () =>
        jsonResponse({
          data: [anna],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <ContactsScreen />
      </StoryProviders>
    );
  },
};

// The list stories below are all about what the TABLE shows, so they share one
// session and it is the smallest one that makes the read legitimate: a contact
// reader. ContactsScreen gates no affordance on a grant — the create button is
// hidden by overlay mode alone, and the archived badge paints off the row's own
// archived_at rather than off a delete verb — so a wider grant would claim
// affordances none of these stories draw.
const contactsReader = meRoute({ contact: ["read"] });

// The empty list: no rows, the "unit.contacts" copy from ListSurface's
// generic empty branch (table.none), nothing else on the page to distract
// from it.
export const ContactsListEmpty: Story = {
  render: () => {
    installFetchStub({
      "GET /me": contactsReader,
      "GET /contacts": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <ContactsScreen />
      </StoryProviders>
    );
  },
};

// A never-resolving GET is the house pattern for a pending story: the query
// never settles, so ListTable stays on its skeleton rows rather than racing
// a timer against Storybook's own render.
export const ContactsListLoading: Story = {
  render: () => {
    installFetchStub({
      "GET /me": contactsReader,
      "GET /contacts": () => new Promise<Response>(() => undefined),
    });
    return (
      <StoryProviders>
        <ContactsScreen />
      </StoryProviders>
    );
  },
};

// A non-2xx body is what useListQuery's isError branch renders: the problem
// detail plus the retry button (listquery.tsx's `problem` slot), never a
// thrown exception the story would surface as a broken render instead.
//
// The session holds no contact grant, which is the same fact the 403 detail
// states — a fixture carrying contact:read here would have the seat and the
// server disagreeing about the very scope the story is showing refused.
export const ContactsListFailed: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({}, { roles: ["rep"] }),
      "GET /contacts": () =>
        jsonResponse(
          { title: "Forbidden", detail: "missing scope contacts:read" },
          403,
        ),
    });
    return (
      <StoryProviders>
        <ContactsScreen />
      </StoryProviders>
    );
  },
};

// has_more: true with a next_cursor is what lights the pager's "next" button
// even though only one page has loaded yet (design-system/listtable.tsx's
// Pager: `disabled={current === lastPage && !hasMore}`), so the load-more
// affordance is that button, not a separate control.
export const ContactsListMorePages: Story = {
  render: () => {
    installFetchStub({
      "GET /me": contactsReader,
      "GET /contacts": () =>
        jsonResponse({
          data: [anna],
          page: { next_cursor: "cursor-2", has_more: true },
        }),
    });
    return (
      <StoryProviders>
        <ContactsScreen />
      </StoryProviders>
    );
  },
};

// A row whose archived_at is set: the warn badge next to the name
// (contacts.tsx's name column) renders off the row's own field, independent
// of the includeArchived toggle's checked state. The checkbox itself starts
// unchecked every render (useListQuery seeds includeArchived: false with no
// prop to override it) and only flips on click, so this story shows the
// badge a toggled-on list would surface without claiming the checkbox is
// lit.
const archivedContact: Contact = {
  id: "p-2",
  full_name: "Mara Voss",
  title: "Former Head of Ops",
  // Every field the contract makes required, not just the ones the row
  // happens to paint: a partial email object still satisfies the response
  // stub, whose body is `unknown`, so the shape only fails where it is
  // typed. Typing it here is what catches the omission at all.
  emails: [
    {
      id: "e-2",
      email: "mara.voss@brandt.example",
      email_type: "work",
      is_primary: true,
      position: 0,
      source: "manual",
      captured_by: "human:u1",
    },
  ],
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-05-01T08:00:00Z",
  updated_at: "2026-07-01T08:00:00Z",
  archived_at: "2026-07-01T08:00:00Z",
};

export const ContactsListArchivedRow: Story = {
  render: () => {
    installFetchStub({
      "GET /me": contactsReader,
      "GET /contacts": () =>
        jsonResponse({
          data: [anna, archivedContact],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <ContactsScreen />
      </StoryProviders>
    );
  },
};

// ContactScreen (the 360 view below) is UNROUTED DEAD CODE: App.tsx routes
// ContactsScreen for the list and ContactPageV2 for the record, never this
// component. Left in place rather than expanded or deleted so a future
// reader does not mistake it for a live surface.
function contactOverviewRoutes() {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }),
    "GET /contacts/p-1": () => jsonResponse(anna),
    "GET /contacts/p-1/strength": () => jsonResponse(dormantStrength),
    "GET /activities": () => jsonResponse({ data: [] }),
    "GET /records/contact/p-1/context": () =>
      jsonResponse({ anchor: { type: "contact", id: "p-1" }, sections: [] }),
  });
}

export const ContactOverview: Story = {
  render: () => {
    contactOverviewRoutes();
    return (
      <StoryProviders>
        <ContactScreen id="p-1" />
      </StoryProviders>
    );
  },
};

// Edit, merge, share and archive are rows of the header's one menu here too,
// the same division the contact page carries — the badges beside the name say
// what the record IS, and the ellipsis holds what may be done to it.
export const ContactOverviewMenu: Story = {
  render: () => {
    contactOverviewRoutes();
    return (
      <StoryProviders>
        <ContactScreen id="p-1" />
      </StoryProviders>
    );
  },
  play: async () => {
    // The panel portals to the body, so it is reached through `screen` and
    // never through a canvas-scoped query.
    await userEvent.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    await screen.findByTestId("edit-record");
  },
};
