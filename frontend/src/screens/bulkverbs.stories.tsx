// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { BulkVerbs } from "./bulkverbs";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The bar the contacts and companies lists show while rows are selected: the
// count, the owner picker with its verb, and archive. Each verb opens the
// preview in `Records/Bulk change`.
const meta: Meta = {
  title: "Records/Bulk verbs",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const roster = {
  data: [
    { id: "u-mila", email: "mila@acme.test", display_name: "Mila Brandt" },
    { id: "u-jonas", email: "jonas@acme.test", display_name: "Jonas Weber" },
  ],
  page: { next_cursor: null, has_more: false },
};

const rows = [
  { id: "c-1", version: 3, label: "Anna Weber" },
  { id: "c-2", version: 7, label: "Ben Ott" },
];

function Bar({ locale }: Readonly<{ locale?: "en" | "de" | "vi" }>) {
  installFetchStub({ "GET /users": () => jsonResponse(roster) });
  return (
    <StoryProviders locale={locale}>
      <div className="lt-bulkbar">
        <BulkVerbs recordType="contact" rows={rows} onDone={() => {}} />
      </div>
    </StoryProviders>
  );
}

export const Selected: Story = { render: () => <Bar /> };

// German, whose owner verb is the longest of the three catalogs.
export const German: Story = { render: () => <Bar locale="de" /> };
