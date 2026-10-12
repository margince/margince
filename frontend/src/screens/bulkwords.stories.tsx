// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DataTable } from "../design-system/datatable";
import { SampleState, SkipReason } from "./bulkwords";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// How the bulk preview names each verb's before and after, and why a record
// was left alone — the words every bulk change shows.
const meta: Meta = {
  title: "Records/Bulk change/Words",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type BulkVerb = components["schemas"]["BulkVerb"];

const verbs: readonly Readonly<{
  verb: BulkVerb;
  before: components["schemas"]["BulkRecordState"];
  after: components["schemas"]["BulkRecordState"];
}>[] = [
  {
    verb: "add_tag",
    before: { owner_id: null, archived: false, tagged: false },
    after: { owner_id: null, archived: false, tagged: true },
  },
  {
    verb: "create_task",
    before: { owner_id: null, archived: false },
    after: { owner_id: null, archived: false, task_id: "a-1" },
  },
  {
    verb: "remove_from_list",
    before: { owner_id: null, archived: false, listed: true },
    after: { owner_id: null, archived: false, listed: false },
  },
  {
    verb: "archive",
    before: { owner_id: null, archived: false },
    after: { owner_id: null, archived: true },
  },
];

export const BeforeAndAfter: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({}) });
    return (
      <StoryProviders>
        <DataTable
          label="Verbs"
          rows={[...verbs]}
          rowKey={(row) => row.verb}
          columns={[
            { key: "verb", header: "Verb", render: (row) => row.verb },
            {
              key: "before",
              header: "Now",
              render: (row) => (
                <SampleState verb={row.verb} state={row.before} />
              ),
            },
            {
              key: "after",
              header: "After",
              render: (row) => (
                <SampleState verb={row.verb} state={row.after} />
              ),
            },
            {
              key: "reason",
              header: "Left alone",
              render: (row) => (
                <SkipReason
                  verb={row.verb}
                  skip={{ id: row.verb, reason: "no_change" }}
                />
              ),
            },
          ]}
        />
      </StoryProviders>
    );
  },
};
