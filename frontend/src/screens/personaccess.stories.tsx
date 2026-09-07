// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { PersonAccess } from "./personaccess";
import { StoryProviders, stubWithSession } from "./story-utils";

// Who may read a captured contact, on the record itself.
//
// The same mark and the same verb the mail drawer draws, because a contact
// private to its owner and a message limited to its participants are the same
// fact about two things — a reader who has learned the mark on one should read
// it on the other. That is why this panel's verb is the text affordance too:
// the fact is the panel's subject, and a filled box beside the badge would be
// the louder half of a line whose point is the badge.

type Person = components["schemas"]["Person"];

const meta: Meta = {
  title: "Records/Person record/Access",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const base: Person = {
  id: "01a05500-0000-7000-8000-0000000000d1",
  full_name: "Dana Buyer",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  version: 7,
};

function Access({ person }: Readonly<{ person: Person }>) {
  stubWithSession({}, { person: ["update"] });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 620 }}>
        <PersonAccess person={person} />
      </div>
    </StoryProviders>
  );
}

/** A contact the organization can read, with the verb that seals it beside the
 *  word that says so. */
export const SharedWithTheTeam: Story = {
  render: () => (
    <Access person={{ ...base, visibility: "workspace", writable: true }} />
  ),
};

/** The same contact private to its owner: the mark is filled, and the verb
 *  names where the contact GOES rather than where it is. */
export const PrivateToItsOwner: Story = {
  render: () => (
    <Access person={{ ...base, visibility: "owner", writable: true }} />
  ),
};

/**
 * A colleague who may read the contact and not rewrite it. The fact and the
 * sentence stay; the verb is simply absent, which is the honest drawing of
 * "you can see that this is limited, and it is not yours to widen".
 */
export const NotYoursToChange: Story = {
  render: () => (
    <Access person={{ ...base, visibility: "owner", writable: false }} />
  ),
};
