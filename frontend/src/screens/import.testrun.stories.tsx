// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { formatFixture } from "./import.fixtures";
import { ProblemTable } from "./import.testrun";
import { ImportWizardPage } from "./import.wizard";
import type { ImportRowIssue } from "./importtypes";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The test run shows the counts per type and every problem with where it is.
// It offers to bring rows with broken links in without the link. The first
// story reaches it the way a reader does. The second opens a staged run by
// address, where no file is in hand to stage again.

const meta: Meta = {
  title: "Settings/Data/Data import/Test run",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const RUN = {
  id: "019ff-run",
  connector: "csv",
  file_name: "crm.zip",
  status: "awaiting_approval",
  checkpoint: 0,
  source: "import_api",
  captured_by: "u-ada",
  created_at: "2026-10-12T09:30:00Z",
  updated_at: "2026-10-12T09:30:00Z",
};

const ISSUES: ImportRowIssue[] = [
  {
    line: 41,
    file: "contacts.csv",
    column: "company_ref",
    kind: "broken_link",
    value: "9921",
    reason: "No company has the ref 9921 in this upload or in Margince.",
  },
  {
    line: 12,
    file: "companies.csv",
    column: "Tier",
    kind: "invalid_value",
    value: "Gold",
    reason: "“Gold” is not one of this field’s allowed values: A, B.",
  },
  {
    line: 3,
    file: "companies.csv",
    column: "owner_email",
    kind: "unknown_owner",
    value: "mai@old-crm.com",
    reason: "No user has this email, so you become the owner.",
  },
];

const REPORT = {
  run_id: RUN.id,
  status: "awaiting_approval",
  rows_read: 266,
  source_key_used: "ref",
  disposition: { created: 230, updated: 26, unchanged: 8, skipped: 2 },
  parts: [
    {
      file: "companies.csv",
      object: "company",
      rows_read: 52,
      disposition: { created: 45, updated: 5, unchanged: 1, skipped: 1 },
    },
    {
      file: "contacts.csv",
      object: "contact",
      rows_read: 214,
      disposition: { created: 185, updated: 21, unchanged: 7, skipped: 1 },
    },
  ],
  issues: ISSUES,
};

const PROFILE = {
  run_id: "019ff-draft",
  source_ref: "ws/import/abc",
  object: "lead",
  file_name: "leads.csv",
  rows_profiled: 1,
  row_count: 1,
  columns: [{ header: "email", fill_rate: 1, samples: ["jo@acme.com"] }],
  suggested_mapping: { email: "email" },
  targets: ["email"],
};

function page(address: string) {
  return () => {
    globalThis.history.replaceState(null, "", address);
    installFetchStub({
      "GET /me": meRoute({ import_run: ["create", "read", "update"] }),
      "GET /imports/format": () => jsonResponse(formatFixture),
      "GET /imports": () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "POST /imports/sources": () => jsonResponse(PROFILE),
      "POST /imports": () => jsonResponse(RUN, 202),
      [`GET /imports/${RUN.id}`]: () => jsonResponse(RUN),
      [`GET /imports/${RUN.id}/report`]: () => jsonResponse(REPORT),
    });
    return (
      <StoryProviders>
        <ImportWizardPage />
      </StoryProviders>
    );
  };
}

async function stage({ canvasElement }: { canvasElement: HTMLElement }) {
  const canvas = within(canvasElement);
  await userEvent.upload(
    await canvas.findByLabelText("Upload your data"),
    new File(["email\n"], "leads.csv"),
  );
  await userEvent.click(
    await canvas.findByRole("button", { name: /Next: map/ }),
  );
  await userEvent.click(canvas.getByRole("button", { name: "Run test" }));
  await canvas.findByRole("link", { name: "problems.csv" });
}

/** Staged here, so the broken links may be brought in without the link. */
export const Problems: Story = {
  render: page("#/settings/import/run"),
  play: stage,
};

export const ProblemsDark: Story = {
  globals: { theme: "dark" },
  render: page("#/settings/import/run"),
  play: stage,
};

/** A staged run opened by its address: no file is in hand to stage again. */
export const OpenedByAddress: Story = {
  render: page(`#/settings/import/run?run=${RUN.id}`),
};

/** The problem list alone, one row per kind of problem. */
export const ProblemList: Story = {
  render: () => (
    <StoryProviders>
      <ProblemTable issues={ISSUES} />
    </StoryProviders>
  ),
};
