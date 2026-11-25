// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { AppLink } from "../../design-system/applink";
import { RecordHrefScope } from "../../lib/recordhref";
import { StoryProviders } from "../../testing/story-utils";
import { WorklistReturnScope } from "./worklist.return";

const meta: Meta<typeof WorklistReturnScope> = {
  title: "Records/Worklist/Return scope",
  component: WorklistReturnScope,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof WorklistReturnScope>;

function Links() {
  return (
    <ul>
      <li>
        <AppLink href="#/contacts/c1">Mareike Vollmer</AppLink>
      </li>
      <li>
        <AppLink href="#/settings/privacy">Privacy case</AppLink>
      </li>
    </ul>
  );
}

/** A record link drawn in the drawer carries its dials; any other link does not. */
export const InTheWorklist: Story = {
  render: () => (
    <StoryProviders>
      <WorklistReturnScope dials={new Map([["queue_scope", "team"]])}>
        <Links />
      </WorklistReturnScope>
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const record = canvas.getByRole("link", { name: "Mareike Vollmer" });
    await expect(record.getAttribute("href")).toContain("from=worklist");
    await expect(record.getAttribute("href")).toContain("queue_scope=team");
    const other = canvas.getByRole("link", { name: "Privacy case" });
    await expect(other.getAttribute("href")).not.toContain("from=worklist");
  },
};

/** Outside any scope a record link keeps its plain address. */
export const OutsideAnyScope: Story = {
  render: () => (
    <StoryProviders>
      <RecordHrefScope href={(href) => href}>
        <Links />
      </RecordHrefScope>
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const record = within(canvasElement).getByRole("link", {
      name: "Mareike Vollmer",
    });
    await expect(record.getAttribute("href")).not.toContain("from=worklist");
  },
};
