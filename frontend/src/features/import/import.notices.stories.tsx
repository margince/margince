// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProblemError } from "../../lib/problem";
import { formatFixture } from "../../testing/import.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { ImportRefusal, UndoConfirm, UndoErrors } from "./import.notices";
import type { ImportFormat } from "./importtypes";

const FIELD_LABELS: Readonly<Record<string, string>> = {
  email: "Email address",
  ref: "Your own key (ref)",
};

// What an undo says about itself, and why the server refused a file. Each is
// posed here, since the wizard reaches it only after several presses.

const meta: Meta = {
  title: "Settings/Data/Data import/Undo and refusal notices",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const ERRORED = [
  {
    object: "company" as const,
    id: "01a04298-2000-7000-8000-000000000001",
    reason: "A contact still references this company.",
  },
  {
    object: "contact" as const,
    id: "01a04298-2000-7000-8000-000000000002",
    reason: "Edited since the import; left exactly as it stood.",
  },
];

/** The rows that could not be put back, named with why. */
export const RowsLeftStanding: Story = {
  render: () => (
    <StoryProviders>
      <UndoErrors rows={ERRORED} />
    </StoryProviders>
  ),
};

/** The same list in dark: tone reaches the heading's ink and the rows read on. */
export const RowsLeftStandingDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <StoryProviders>
      <UndoErrors rows={ERRORED} />
    </StoryProviders>
  ),
};

/** A clean reversal names no row. */
export const Quiet: Story = {
  render: () => (
    <StoryProviders>
      <UndoErrors rows={[]} />
    </StoryProviders>
  ),
};

/** What an undo will do, asked before it starts, in reverse import order. */
export const UndoAsked: Story = {
  render: () => (
    <StoryProviders>
      <UndoConfirm
        open
        order={["company", "contact", "lead"]}
        pending={false}
        error={null}
        onClose={() => undefined}
        onConfirm={() => undefined}
      />
    </StoryProviders>
  ),
};

// The labels the server's format gives a core column in the reader's language.
const LABELLED: ImportFormat = {
  ...formatFixture,
  files: formatFixture.files.map((file) => ({
    ...file,
    columns: file.columns.map((column) => ({
      ...column,
      field_label: FIELD_LABELS[column.name] ?? column.name,
    })),
  })),
};

function refusal(
  field: string,
  code: string,
  params?: Record<string, string>,
  max?: number,
) {
  return new ProblemError({
    status: 422,
    code: "validation_error",
    detail:
      "The server’s own sentence, kept for a code this build does not know.",
    details: {
      errors: [
        {
          field,
          code,
          message:
            "The server’s own sentence, kept for a code this build does not know.",
          ...(max === undefined ? null : { max }),
          ...(params ? { params } : null),
        },
      ],
    },
  });
}

/** A line of a CSV inside a ZIP the reader can go and fix. */
export const RefusedLine: Story = {
  render: () => (
    <StoryProviders>
      <ImportRefusal
        title="File could not be read"
        error={refusal("file", "unreadable", {
          reason: "field_count",
          line: "1204",
          file: "contacts.csv",
        })}
      />
    </StoryProviders>
  ),
};

/** Two files of one type in a ZIP, both named. */
export const RefusedTwoFilesOneType: Story = {
  render: () => (
    <StoryProviders>
      <ImportRefusal
        title="File could not be read"
        error={refusal("file", "bundle_refused", {
          reason: "two_files_one_type",
          other_file: "export/contacts.csv",
          file: "CONTACTS.CSV",
          object: "contact",
        })}
      />
    </StoryProviders>
  ),
};

/** A file too long for one test run, with its count and the server's limit. */
export const RefusedTooManyRows: Story = {
  render: () => (
    <StoryProviders>
      <ImportRefusal
        title="File could not be read"
        error={refusal("file", "too_many_rows", { rows: "21480" }, 15000)}
      />
    </StoryProviders>
  ),
};

/** No column tells the rows apart: the key field and your own key, by label. */
export const RefusedNoKeyColumn: Story = {
  render: () => (
    <StoryProviders>
      <ImportRefusal
        title="Test run did not start"
        format={LABELLED}
        error={refusal("source_key", "unmappable", {
          object: "contact",
          target: "email",
          ref_target: "ref",
        })}
        keepsDraft
      />
    </StoryProviders>
  ),
};

export const RefusedNoKeyColumnDark: Story = {
  globals: { theme: "dark" },
  render: RefusedNoKeyColumn.render,
};

/** A code this build does not know keeps the server's own sentence. */
export const RefusedUnknownCode: Story = {
  render: () => (
    <StoryProviders>
      <ImportRefusal
        title="File could not be read"
        error={refusal("file", "from_a_newer_server")}
      />
    </StoryProviders>
  ),
};
