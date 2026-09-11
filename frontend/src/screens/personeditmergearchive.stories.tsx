// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { components } from "../api/schema";
import { Button, OverflowMenu } from "../design-system/atoms";
import { useObjectCustomFields } from "./customfields.form";
import { PersonEditMergeArchive } from "./personeditmergearchive";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

type Person = components["schemas"]["Person"];

// The record's core write verbs as both callers draw them: rows of an
// OverflowMenu, worded, with the destructive one last. The menu is what the
// story is FOR — these three have no standalone face any more, so a story
// showing them in a bare row would document a shape the product does not
// have.
const meta: Meta = {
  title: "Records/Person write verbs",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const ANNA: Person = {
  id: "p-1",
  full_name: "Anna Weber",
  title: "Head of Procurement",
  emails: [
    {
      id: "e-1",
      person_id: "p-1",
      email: "anna.weber@brandt.example",
      email_type: "work",
      is_primary: true,
      position: 0,
      source: "manual",
      captured_by: "human:u1",
    },
  ],
  version: 3,
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-05-01T08:00:00Z",
  updated_at: "2026-07-01T08:00:00Z",
};

// The real hook rather than a hand-built ObjectCustomFields: the schema read
// is what decides which rows the edit form carries, and a fixture standing in
// for it would document a form the product does not build.
function PersonMenu({
  person,
  disabledReasonId,
}: Readonly<{ person: Person; disabledReasonId?: string }>) {
  const cf = useObjectCustomFields("person");
  return (
    <OverflowMenu label="More actions">
      <PersonEditMergeArchive
        person={person}
        cf={cf}
        disabledReasonId={disabledReasonId}
        overlay={false}
        beforeArchive={
          // What `beforeArchive` is for: the caller's own quieter rows, seated
          // ahead of the destructive one. On the record page this is Share,
          // Full history and Research.
          <Button small>Share</Button>
        }
      />
    </OverflowMenu>
  );
}

const ROUTES = {
  "GET /me": meRoute({ person: ["read", "update"] }),
  "GET /custom-fields": () => jsonResponse({ data: [] }),
};

export const OpenMenu: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <PersonMenu person={ANNA} />
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

// An archived record is read-only, and the rows say so rather than
// disappearing: a missing control states nothing about the record, a refused
// one names the reason (STATE-4a). The sentence belongs to the page, and each
// row points at it.
export const RefusedByArchive: Story = {
  render: () => {
    installFetchStub(ROUTES);
    return (
      <StoryProviders>
        <PersonMenu
          person={{ ...ANNA, archived_at: "2026-07-13T00:00:00Z" }}
          disabledReasonId="person-archived"
        />
        <p id="person-archived" className="t-caption">
          This contact is archived, so it takes no changes.
        </p>
      </StoryProviders>
    );
  },
  play: async () => {
    await userEvent.click(screen.getByRole("button", { name: "More actions" }));
    await screen.findByTestId("edit-record");
  },
};
