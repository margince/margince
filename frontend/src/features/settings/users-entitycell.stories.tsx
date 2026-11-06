// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import type { LegalEntity } from "../../lib/legalentities.queries";
import { StoryProviders } from "../../testing/story-utils";
import { MemberEntity } from "./users-entitycell";

// One member's legal entity on the roster, as a name at rest. An admin who may
// move members gets a chooser of the live entities on press.

const BASE = {
  address: {},
  member_count: 0,
  version: 1,
  created_at: "2026-01-12T09:00:00Z",
  updated_at: "2026-01-12T09:00:00Z",
};
const ENTITIES: LegalEntity[] = [
  { ...BASE, id: "le-default", is_default: true, display_name: "Brandt" },
  { ...BASE, id: "le-vienna", is_default: false, display_name: "Brandt Wien" },
];

const DANA: components["schemas"]["User"] = {
  id: "u-2",
  email: "dana@brandt.example",
  display_name: "Dana Kessler",
  timezone: "",
  status: "active",
  is_agent: false,
};

function cell(current: string, canAssign: boolean) {
  return () => (
    <StoryProviders>
      <MemberEntity
        member={DANA}
        entities={ENTITIES}
        current={current}
        canAssign={canAssign}
      />
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/People/Members/Legal entity picker",
  component: MemberEntity,
  args: {
    member: DANA,
    entities: ENTITIES,
    current: "le-vienna",
    canAssign: true,
  },
} satisfies Meta<typeof MemberEntity>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Assignable: Story = { render: cell("le-vienna", true) };

// An admin without the legal entity grant reads the name as words.
export const ReadOnly: Story = { render: cell("le-default", false) };

export const ChooserOpen: Story = {
  render: cell("le-vienna", true),
  play: async ({ canvasElement }) => {
    const name = /Dana Kessler’s legal entity/;
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name }),
    );
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      { name },
    );
    await userEvent.click(within(dialog).getByRole("combobox", { name }));
  },
};
