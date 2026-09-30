// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useT } from "../i18n";
import { MemberRows, memberName } from "./listmembers";
import { listsMe, members, shortlist } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// A list's members with what a reader can do to them: tick rows for the bulk
// bar, select every member, or export the list.
const meta: Meta = {
  title: "Records/List members",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function Members() {
  const t = useT();
  return (
    <MemberRows
      list={shortlist}
      source="company"
      onOpen={() => {}}
      columns={[
        {
          key: "name",
          header: t("lists.col.name"),
          fixed: true,
          cell: (row) => memberName(row, t),
        },
      ]}
    />
  );
}

export const ShortlistMembers: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <Members />
      </StoryProviders>
    );
  },
};
