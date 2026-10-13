// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ImportDropzone } from "./import.files";
import { formatFixture, recentRunsFixture } from "./import.fixtures";
import { ImportWizardPage } from "./import.wizard";
import { ImportMappingTable } from "./importmapping";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The wizard's first three steps: drop a file, check the type of each file in
// it, and map each file's columns on its own tab. A story reaches a step by
// dropping a ZIP whose profile is stubbed.

const meta: Meta = {
  title: "Settings/Data/Data import/Files and map",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const ZIP_PROFILE = {
  run_id: "019ff-draft",
  source_ref: "ws/import/zip",
  file_name: "pipedrive-export.zip",
  suggested_source_name: "pipedrive-export",
  rows_profiled: 0,
  row_count: 266,
  columns: [],
  suggested_mapping: {},
  targets: [],
  parts: [
    {
      file: "companies.csv",
      object: "company",
      rows_profiled: 52,
      row_count: 52,
      columns: [
        { header: "ref", fill_rate: 1, samples: ["CO-1", "CO-2"] },
        { header: "display_name", fill_rate: 1, samples: ["Acme GmbH"] },
        { header: "Tier", fill_rate: 0.6, samples: ["A", "B"] },
      ],
      suggested_mappings: {
        company: { ref: "ref", display_name: "display_name", Tier: "cf_tier" },
      },
    },
    {
      file: "contacts.csv",
      object: "contact",
      rows_profiled: 214,
      row_count: 214,
      columns: [
        { header: "ref", fill_rate: 1, samples: ["P-1"] },
        { header: "email", fill_rate: 0.98, samples: ["jo@acme.com"] },
        { header: "company_ref", fill_rate: 0.9, samples: ["CO-1"] },
        { header: "Notes", fill_rate: 0.04, samples: [] },
      ],
      suggested_mappings: {
        contact: { ref: "ref", email: "email", company_ref: "company_ref" },
      },
    },
  ],
  ignored: [{ file: "products.csv", reason: "unknown_name" }],
};

function wizard() {
  installFetchStub({
    "GET /me": meRoute({ import_run: ["create", "read", "update"] }),
    "GET /imports/format": () => jsonResponse(formatFixture),
    "GET /imports": () =>
      jsonResponse({ data: recentRunsFixture, page: { has_more: false } }),
    "POST /imports/sources": () => jsonResponse(ZIP_PROFILE),
  });
  return (
    <StoryProviders>
      <ImportWizardPage />
    </StoryProviders>
  );
}

async function dropZip(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await userEvent.upload(
    await canvas.findByLabelText("Upload your data"),
    new File(["PK"], "pipedrive-export.zip"),
  );
  await canvas.findByRole("combobox", {
    name: "Record type for companies.csv",
  });
}

/** The drop zone the hub and the first step share. */
export const DropZone: Story = {
  render: () => (
    <StoryProviders>
      <ImportDropzone sizeLimitMb={10} onPick={() => undefined} />
    </StoryProviders>
  ),
};

/** The first step, before a file. */
export const Upload: Story = { render: wizard };

/** Each file with its detected type, the file left out, and the source name. */
export const CheckingFiles: Story = {
  render: wizard,
  play: ({ canvasElement }) => dropZip(canvasElement),
};

export const CheckingFilesDark: Story = {
  globals: { theme: "dark" },
  render: wizard,
  play: ({ canvasElement }) => dropZip(canvasElement),
};

/** One tab per file in import order; a chosen field shows its marks. */
export const MappingColumns: Story = {
  render: wizard,
  play: async ({ canvasElement }) => {
    await dropZip(canvasElement);
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: /Next: map 2 files/ }),
    );
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: /Contacts/ }),
    );
  },
};

/** The table alone, with a custom field chosen and a column left out. */
export const MappingTable: Story = {
  render: () => (
    <StoryProviders>
      <ImportMappingTable
        columns={ZIP_PROFILE.parts[0]?.columns ?? []}
        rowsProfiled={52}
        targets={formatFixture.files[0]?.columns ?? []}
        file="companies.csv"
        mapping={{ ref: "ref", Tier: "cf_tier" }}
        locked={false}
        onChange={() => undefined}
      />
    </StoryProviders>
  ),
};
