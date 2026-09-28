// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { BulkChangeDialog, type BulkChangeRequest } from "./bulkchange";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The preview a bulk verb opens before anything is written, in each state the
// server can answer with.
const meta: Meta = {
  title: "Records/Bulk change",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type BulkChangePreview = components["schemas"]["BulkChangePreview"];

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
  { id: "c-3", version: 2, label: "Clara Ruiz" },
  { id: "c-4", version: 1, label: "Dana Kohl" },
];

const reassign: BulkChangeRequest = {
  recordType: "contact",
  verb: "reassign_owner",
  ownerId: "u-jonas",
  openId: "story",
  rows,
};

const archive: BulkChangeRequest = {
  ...reassign,
  recordType: "company",
  verb: "archive",
  ownerId: undefined,
  rows: [
    { id: "co-1", version: 4, label: "Brandt GmbH" },
    { id: "co-2", version: 2, label: "Ott Logistik" },
  ],
};

const reassignPreview: BulkChangePreview = {
  record_type: "contact",
  verb: "reassign_owner",
  count: 2,
  affected: ["c-1", "c-2"],
  excluded: [],
  sample: [
    {
      id: "c-1",
      label: "Anna Weber",
      before: { owner_id: "u-mila", archived: false },
      after: { owner_id: "u-jonas", archived: false },
    },
    {
      id: "c-2",
      label: "Ben Ott",
      before: { owner_id: null, archived: false },
      after: { owner_id: "u-jonas", archived: false },
    },
  ],
  requires_confirmation: false,
};

function Dialog({
  request,
  preview,
  locale,
}: Readonly<{
  request: BulkChangeRequest;
  preview: BulkChangePreview;
  locale?: "en" | "de" | "vi";
}>) {
  installFetchStub({
    "GET /users": () => jsonResponse(roster),
    "POST /bulk/preview": () => jsonResponse(preview),
  });
  return (
    <StoryProviders locale={locale}>
      <BulkChangeDialog
        request={request}
        onClose={() => {}}
        onDone={() => {}}
      />
    </StoryProviders>
  );
}

export const ReassignSample: Story = {
  render: () => <Dialog request={reassign} preview={reassignPreview} />,
};

export const WithExclusions: Story = {
  render: () => (
    <Dialog
      request={reassign}
      preview={{
        ...reassignPreview,
        excluded: [
          { id: "c-3", reason: "no_change" },
          { id: "c-4", reason: "changed_since_preview" },
        ],
      }}
    />
  ),
};

// More than ten records: the server hands back a token, and the dialog says the
// change is large before the reader confirms it.
const manyRows = Array.from({ length: 25 }, (_, index) => ({
  id: `c-${index + 1}`,
  version: 1,
  label: `Contact ${index + 1}`,
}));

export const NeedsConfirmation: Story = {
  render: () => (
    <Dialog
      request={{ ...reassign, rows: manyRows }}
      preview={{
        ...reassignPreview,
        count: 23,
        requires_confirmation: true,
        confirm_token: "story-token",
        expires_at: "2026-09-27T12:10:00Z",
      }}
    />
  ),
};

export const NothingWouldChange: Story = {
  render: () => (
    <Dialog
      request={reassign}
      preview={{
        ...reassignPreview,
        count: 0,
        affected: [],
        sample: [],
        excluded: rows.map((row) => ({
          id: row.id,
          reason: "no_change" as const,
        })),
      }}
    />
  ),
};

export const ArchiveSample: Story = {
  render: () => (
    <Dialog
      request={archive}
      preview={{
        record_type: "company",
        verb: "archive",
        count: 1,
        affected: ["co-2"],
        excluded: [{ id: "co-1", reason: "anchor_company" }],
        sample: [
          {
            id: "co-2",
            label: "Ott Logistik",
            before: { owner_id: "u-mila", archived: false },
            after: { owner_id: "u-mila", archived: true },
          },
        ],
        requires_confirmation: false,
      }}
    />
  ),
};

// German, whose reason texts and title are the longest.
export const GermanWithExclusions: Story = {
  render: () => (
    <Dialog
      locale="de"
      request={reassign}
      preview={{
        ...reassignPreview,
        excluded: [
          { id: "c-3", reason: "not_writable" },
          { id: "c-4", reason: "refused", message: "Legal Hold aktiv." },
        ],
      }}
    />
  ),
};
