// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { formatFixture } from "../../testing/import.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ProblemTable } from "./import.testrun";
import { ImportWizardPage } from "./import.wizard";
import type { ImportRowIssue } from "./importtypes";

// The test run shows the counts per type and every problem with where it is.
// It offers to bring rows with broken links in without the link. The first
// story reaches it the way a reader does. The second opens a staged run by
// address, where no file is in hand to stage again. Then a test still running,
// and one the worker refused.

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
  version: 1,
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
    reason:
      "No user has this email, so you become the owner; a lead stays unassigned. Rows naming it: 1.",
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

function page(address: string, current: object = RUN) {
  return () => {
    globalThis.history.replaceState(null, "", address);
    // The draft answers as itself until the worker is started, then as the
    // run the test produced.
    //
    // Run test saves the mapping, reads the version off that answer, then
    // posts to validate. A stub answering none of those leaves the wizard on
    // the map step with nothing to wait for.
    let tested = false;
    installFetchStub({
      "GET /me": meRoute({ import_run: ["create", "read", "update"] }),
      "GET /imports/format": () => jsonResponse(formatFixture),
      "GET /imports": () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "POST /imports/sources": () => jsonResponse(PROFILE),
      "POST /imports": () => jsonResponse(current, 202),
      [`GET /imports/${PROFILE.run_id}`]: () =>
        jsonResponse(
          tested
            ? { ...current, id: PROFILE.run_id }
            : { ...RUN, id: PROFILE.run_id, status: "pending" },
        ),
      [`PATCH /imports/${PROFILE.run_id}`]: () =>
        jsonResponse({ ...RUN, id: PROFILE.run_id, status: "pending" }),
      [`POST /imports/${PROFILE.run_id}/validate`]: () => {
        tested = true;
        return jsonResponse({ ...current, id: PROFILE.run_id }, 202);
      },
      [`GET /imports/${PROFILE.run_id}/report`]: () => jsonResponse(REPORT),
      [`GET /imports/${RUN.id}`]: () => jsonResponse(current),
      [`GET /imports/${RUN.id}/report`]: () => jsonResponse(REPORT),
    });
    return (
      <StoryProviders>
        <ImportWizardPage />
      </StoryProviders>
    );
  };
}

async function runTest(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await userEvent.upload(
    await canvas.findByLabelText("Upload your data"),
    // The file the screen's own testkit uploads, so this play drives the
    // wizard the way its unit tests do.
    new File(["Email\nada@x.test\n"], "leads.csv"),
  );
  await userEvent.click(
    await canvas.findByRole("button", { name: /Next: map/ }),
  );
  await userEvent.click(canvas.getByRole("button", { name: "Run test" }));
  return canvas;
}

async function stage({ canvasElement }: { canvasElement: HTMLElement }) {
  const canvas = await runTest(canvasElement);
  await canvas.findByRole("link", { name: "problems.csv" });
}

const NO_HEADER = "The file has no header row.";

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

/** A run the worker is still testing, which the reader can stop. */
export const Testing: Story = {
  render: page(`#/settings/import/run?run=${RUN.id}`, {
    ...RUN,
    status: "validating",
  }),
};

/** A test the worker refused: back on the map with the reason. */
export const TestRefused: Story = {
  render: page("#/settings/import/run", {
    ...RUN,
    status: "pending",
    error: NO_HEADER,
  }),
  play: async ({ canvasElement }) => {
    const canvas = await runTest(canvasElement);
    await canvas.findByText(NO_HEADER);
  },
};

/** The problem list alone, one row per kind of problem. */
export const ProblemList: Story = {
  render: () => (
    <StoryProviders>
      <ProblemTable issues={ISSUES} />
    </StoryProviders>
  ),
};
