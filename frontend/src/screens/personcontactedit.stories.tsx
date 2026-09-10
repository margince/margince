// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { EditContactMethodsModal } from "./personcontactedit";
import { StoryProviders } from "./story-utils";

// The staged editor for a person's emails and numbers, open — its own gallery
// so the modal has a render-gate story beside the component it covers
// (frontend/AGENTS.md: stories live beside their component). PersonRail's own
// stories (personpage.stories.tsx) still cover the rail's "Edit contact
// methods" button that opens this modal in the live page; this file is the
// modal on its own.

const meta: Meta = {
  title: "Records/Person record/Contact methods editor",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// A couple of each row, so the gallery shows what a reader edits: a primary
// and a secondary email, a primary and a secondary phone, each staged from
// the row the modal seeds itself from.
const person: components["schemas"]["Person"] = {
  id: "p-1",
  full_name: "Dana Buyer",
  first_name: "Dana",
  last_name: "Buyer",
  title: "Head of Fleet",
  emails: [
    {
      id: "pe-1",
      person_id: "p-1",
      email: "dana@brandt.example",
      email_type: "work",
      is_primary: true,
      position: 0,
      source: "manual",
      captured_by: "human:u-1",
    },
    {
      id: "pe-2",
      person_id: "p-1",
      email: "dana.buyer@gmail.example",
      email_type: "personal",
      is_primary: false,
      position: 1,
      source: "manual",
      captured_by: "human:u-1",
    },
  ],
  phones: [
    {
      id: "pp-1",
      person_id: "p-1",
      phone: "+493012345678",
      phone_type: "work",
      is_primary: true,
      position: 0,
      source: "manual",
      captured_by: "human:u-1",
    },
    {
      id: "pp-2",
      person_id: "p-1",
      phone: "+491701234567",
      phone_type: "mobile",
      is_primary: false,
      position: 1,
      source: "manual",
      captured_by: "human:u-1",
    },
  ],
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
};

export const ContactMethodsEditor: Story = {
  render: () => (
    <StoryProviders>
      <EditContactMethodsModal open onClose={() => {}} person={person} />
    </StoryProviders>
  ),
};
