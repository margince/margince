// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { IdentityFact, IdentityLine } from "../design-system/identityline";
import { RecordAccess } from "./recordaccess";
import { StoryProviders, stubWithSession } from "./story-utils";

type Contact = components["schemas"]["Contact"];
type Company = components["schemas"]["Company"];

const meta: Meta = {
  title: "Records/Record access",
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
        <RecordAccess kind="contact" record={contact} />
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

const account: Company = {
  id: "01a05500-0000-7000-8000-0000000000c1",
  display_name: "Weber GmbH",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  version: 4,
};

function AccountAccess({ company }: Readonly<{ company: Company }>) {
  stubWithSession({}, { company: ["update"] });
  return (
    <StoryProviders>
      <IdentityLine separator="space">
        <IdentityFact quiet>Owner: Alex</IdentityFact>
        <RecordAccess kind="company" record={company} />
      </IdentityLine>
    </StoryProviders>
  );
}

/** The same mark on an account, which had none before: a company capture
 *  minted from an unjudged message is the owner's alone, and the header said
 *  nothing about it. */
export const AccountPrivateToItsOwner: Story = {
  render: () => (
    <AccountAccess
      company={{ ...account, visibility: "owner", writable: true }}
    />
  ),
};

export const AccountSharedWithTheTeam: Story = {
  render: () => (
    <AccountAccess
      company={{ ...account, visibility: "workspace", writable: true }}
    />
  ),
};
