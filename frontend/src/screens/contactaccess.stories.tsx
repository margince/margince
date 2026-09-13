// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { IdentityFact, IdentityLine } from "../design-system/identityline";
import { ContactAccess } from "./contactaccess";
import { StoryProviders, stubWithSession } from "./story-utils";

type Contact = components["schemas"]["Contact"];

const meta: Meta = {
  title: "Records/Contact record/Access",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const base: Contact = {
  id: "01a05500-0000-7000-8000-0000000000d1",
  full_name: "Dana Buyer",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  version: 7,
};

function Access({ contact }: Readonly<{ contact: Contact }>) {
  stubWithSession({}, { contact: ["update"] });
  return (
    <StoryProviders>
      <IdentityLine separator="space">
        <IdentityFact quiet>Owner: Alex</IdentityFact>
        <ContactAccess contact={contact} />
      </IdentityLine>
    </StoryProviders>
  );
}

/** A contact the company can read, with the verb that seals it beside the
 *  word that says so. */
export const SharedWithTheTeam: Story = {
  render: () => (
    <Access contact={{ ...base, visibility: "workspace", writable: true }} />
  ),
};

/** The same contact private to its owner: the mark is filled, and the verb
 *  names where the contact GOES rather than where it is. */
export const PrivateToItsOwner: Story = {
  render: () => (
    <Access contact={{ ...base, visibility: "owner", writable: true }} />
  ),
};

/**
 * A colleague who may read the contact and not rewrite it. The fact and the
 * sentence stay; the verb is simply absent, which is the honest drawing of
 * "you can see that this is limited, and it is not yours to widen".
 */
export const NotYoursToChange: Story = {
  render: () => (
    <Access contact={{ ...base, visibility: "owner", writable: false }} />
  ),
};

export const Archived: Story = {
  render: () => (
    <Access
      contact={{
        ...base,
        visibility: "workspace",
        writable: true,
        archived_at: "2026-08-02T00:00:00Z",
      }}
    />
  ),
};
