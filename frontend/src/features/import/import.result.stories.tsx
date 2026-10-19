// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { formatFixture } from "./import.fixtures";
import { PartProgressList } from "./import.result";
import { ImportWizardPage } from "./import.wizard";
import type { ImportRun, ImportRunPart } from "./importtypes";

// The import while it runs and what it did. One bar per file, the stop and its
// resume, the counts with Undo, and what an undo kept.

const meta: Meta = {
  title: "Settings/Data/Data import/Import and result",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const PARTS: ImportRunPart[] = [
  { file: "companies.csv", object: "company", row_count: 52 },
  { file: "contacts.csv", object: "contact", row_count: 214 },
];

function run(
  status: ImportRun["status"],
  extra: Partial<ImportRun> = {},
): ImportRun {
  return {
    id: "019ff-run",
    connector: "csv",
    file_name: "crm.zip",
    status,
    checkpoint: 0,
    version: 2,
    source_released: false,
    source: "import_api",
    captured_by: "u-ada",
    parts: PARTS,
    created_at: "2026-10-12T09:30:00Z",
    updated_at: "2026-10-12T09:32:00Z",
    ...extra,
  };
}

function report(status: string, extra: Record<string, unknown> = {}) {
  return {
    run_id: "019ff-run",
    status,
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
    links: { offered: 206, applied: 205 },
    issues: [
      {
        line: 41,
        file: "contacts.csv",
        column: "company_ref",
        kind: "broken_link",
        value: "9921",
        reason: "No company has the ref 9921 in this upload or in Margince.",
      },
    ],
    ...extra,
  };
}

function page(
  theRun: ReturnType<typeof run>,
  theReport: ReturnType<typeof report>,
) {
  return () => {
    globalThis.history.replaceState(
      null,
      "",
      `#/settings/import/run?run=${theRun.id}`,
    );
    installFetchStub({
      "GET /me": meRoute({ import_run: ["create", "read", "update"] }),
      "GET /imports/format": () => jsonResponse(formatFixture),
      [`GET /imports/${theRun.id}`]: () => jsonResponse(theRun),
      [`GET /imports/${theRun.id}/report`]: () => jsonResponse(theReport),
    });
    return (
      <StoryProviders>
        <ImportWizardPage />
      </StoryProviders>
    );
  };
}

/** Companies done, contacts part-way: the checkpoint runs across both. */
export const Importing: Story = {
  render: page(run("running", { checkpoint: 120 }), report("running")),
};

/** A run that stopped, with its cause and the resume. */
export const Stopped: Story = {
  render: page(
    run("failed", {
      checkpoint: 120,
      error: "The import stopped at row 68 of contacts.csv.",
    }),
    report("failed"),
  ),
};

export const Complete: Story = {
  render: page(run("complete", { checkpoint: 266 }), report("complete")),
};

export const CompleteDark: Story = {
  globals: { theme: "dark" },
  render: page(run("complete", { checkpoint: 266 }), report("complete")),
};

/** An undo that kept an edited record and a company that a deal still names. */
export const Undone: Story = {
  render: page(
    run("undone", { checkpoint: 266 }),
    report("undone", {
      undo: {
        run_id: "019ff-run",
        status: "undone",
        reversed_count: 228,
        kept: [
          {
            object: "contact",
            id: "01a04298-2000-7000-8000-000000000002",
            reason_code: "undo_edited",
          },
          {
            object: "company",
            id: "01a04298-2000-7000-8000-000000000001",
            reason_code: "undo_still_referenced",
            params: { deals: "1", contacts: "0" },
          },
        ],
        errored: [],
      },
    }),
  ),
};

/** The bars alone, between files. */
export const ProgressBars: Story = {
  render: () => (
    <StoryProviders>
      <PartProgressList
        run={run("running", { checkpoint: 120 })}
        order={formatFixture.order}
      />
    </StoryProviders>
  ),
};
