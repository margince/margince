// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { TagsPanel } from "./tagspanel";

// The tags panel draws four states, and three of them look alike from the
// code: ready, empty and withheld all answer 200 with a list. What separates
// them is what the panel SAYS, which is why each has a story of its own rather
// than one story with a prop.

const meta: Meta = {
  title: "Records/Tags panel",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const ORG = "01a06151-0000-7000-8000-000000000001";

type PanelTag = {
  tag_id: string;
  name: string;
  color?: string;
  archived: boolean;
  assigned_at: string;
  assigned_by?: { display_name: string; kind: string };
};

function Panel({
  tags,
  withheld = false,
  canEdit = true,
}: Readonly<{ tags: PanelTag[]; withheld?: boolean; canEdit?: boolean }>) {
  installFetchStub({
    [`GET /records/organization/${ORG}/tags`]: () =>
      jsonResponse({ data: tags, withheld }),
  });
  return (
    <StoryProviders>
      <TagsPanel entityType="organization" entityID={ORG} canEdit={canEdit} />
    </StoryProviders>
  );
}

const KEY_ACCOUNT: PanelTag = {
  tag_id: "t-1",
  name: "Key Account",
  color: "amber",
  archived: false,
  assigned_at: "2026-03-03T10:00:00Z",
  assigned_by: { display_name: "Lena Fischer", kind: "human" },
};

const CHURN_RISK: PanelTag = {
  tag_id: "t-2",
  name: "Churn Risk",
  color: "rose",
  archived: false,
  assigned_at: "2026-02-01T10:00:00Z",
  assigned_by: { display_name: "Lars", kind: "human" },
};

export const Ready: Story = {
  render: () => <Panel tags={[KEY_ACCOUNT, CHURN_RISK]} />,
};

/**
 * Teaches rather than reporting: a bare "No tags" says what is absent, this
 * says what the feature is for.
 */
export const Empty: Story = {
  render: () => <Panel tags={[]} />,
};

/**
 * Withheld is NOT empty. A caller who may read the record and not the
 * vocabulary is told the words are hidden, because "no tags" is a claim about
 * the record that nobody established.
 */
export const Withheld: Story = {
  render: () => <Panel tags={[]} withheld />,
};

/**
 * An archived word stays ON the record it was applied to and draws muted:
 * retiring a tag stops it being applied, it does not un-tag history.
 */
export const WithAnArchivedWord: Story = {
  render: () => (
    <Panel
      tags={[
        KEY_ACCOUNT,
        {
          tag_id: "t-3",
          name: "Trade Fair 2025",
          archived: true,
          assigned_at: "2025-09-01T10:00:00Z",
          assigned_by: { display_name: "Lena Fischer", kind: "human" },
        },
      ]}
    />
  ),
};

/**
 * Past four, the rest fold behind "+N more": a record with twenty tags would
 * otherwise push everything below it off the rail.
 */
export const Overflowing: Story = {
  render: () => (
    <Panel
      tags={[
        "Key Account",
        "Churn Risk",
        "Inbound",
        "Parked",
        "DACH",
        "EV programme",
      ].map((name, i) => ({
        tag_id: `t-${i}`,
        name,
        color: ["teal", "amber", "rose", "slate"][i % 4],
        archived: false,
        assigned_at: "2026-03-03T10:00:00Z",
        assigned_by: { display_name: "Lena Fischer", kind: "human" },
      }))}
    />
  ),
};

/** A reader who may not change the record sees the words and no verbs. */
export const ReadOnly: Story = {
  render: () => <Panel tags={[KEY_ACCOUNT]} canEdit={false} />,
};

/**
 * An assignment written before the product recorded WHO shows the date with no
 * name, rather than crediting somebody who may not have chosen it.
 */
export const AssignerUnknown: Story = {
  render: () => (
    <Panel
      tags={[
        {
          tag_id: "t-4",
          name: "Imported",
          color: "slate",
          archived: false,
          assigned_at: "2025-01-01T10:00:00Z",
        },
      ]}
    />
  ),
};
