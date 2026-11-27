// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { en } from "../../i18n/en";
import type { Locale } from "../../i18n/locale";
import { ProblemError } from "../../lib/problem";
import { formatFixture } from "../../testing/import.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { ImportRefusal, useRunStopSentence } from "./import.notices";
import type { ImportFormat, ImportRun } from "./importtypes";

afterEach(cleanup);

const busy = new ProblemError({
  status: 409,
  code: "import_source_busy",
  details: { run_id: "run-1" },
});

function refusal(keepsDraft: boolean) {
  render(
    <StoryProviders>
      <ImportRefusal title="Refused" error={busy} keepsDraft={keepsDraft} />
    </StoryProviders>,
  );
}

describe("a busy source name refusal", () => {
  // A stage leaves the upload as a draft, and the reader is told where it waits.
  it("says where the draft waits when the refused call left one", () => {
    refusal(true);
    expect(screen.getByText(en["import.sourceBusy"])).toBeVisible();
    expect(screen.getByText(en["import.sourceBusyDraft"])).toBeVisible();
  });

  // Undo, cancel and approve leave no draft, so the sentence would be false.
  it("says nothing of a draft when the refused call left none", () => {
    refusal(false);
    expect(screen.getByText(en["import.sourceBusy"])).toBeVisible();
    expect(screen.queryByText(en["import.sourceBusyDraft"])).toBeNull();
  });
});

type Fault = Readonly<{
  field: string;
  code: string;
  max?: number;
  params?: Record<string, string>;
}>;

const SERVER_SENTENCE = "The server's own sentence.";

function refused(fault: Fault, format?: ImportFormat, locale: Locale = "en") {
  const error = new ProblemError({
    status: 422,
    code: "validation_error",
    detail: SERVER_SENTENCE,
    details: { errors: [{ ...fault, message: SERVER_SENTENCE }] },
  });
  render(
    <StoryProviders locale={locale}>
      <ImportRefusal
        title="Test run did not start"
        error={error}
        format={format}
      />
    </StoryProviders>,
  );
}

// The words under the callout's title, the title left out.
function body(): string {
  const title = screen.getByText(/did not start|too long|zu lang/);
  return (title.closest(".callout")?.textContent ?? "").replace(
    title.textContent ?? "",
    "",
  );
}

const formatWithLabels: ImportFormat = {
  ...formatFixture,
  files: formatFixture.files.map((file) => ({
    ...file,
    columns: file.columns.map((column) =>
      column.name === "email"
        ? { ...column, field_label: "Email address" }
        : column.name === "ref"
          ? { ...column, field_label: "Your own key (ref)" }
          : column,
    ),
  })),
};

describe("a refused file says what is wrong in the reader's words", () => {
  it("names a file that is not UTF-8, and the ZIP entry it is", () => {
    refused({ field: "file", code: "not_utf8", params: { file: "leads.csv" } });
    expect(body()).toBe(
      "In “leads.csv”: The file is not saved as UTF-8. Save it as CSV UTF-8 and upload it again.",
    );
  });

  it("names the line with more fields than the header", () => {
    refused({
      field: "file",
      code: "unreadable",
      params: { reason: "field_count", line: "1204" },
    });
    expect(body()).toBe(
      "Line 1,204 has more or fewer fields than the header row. Fix that line and upload the file again.",
    );
  });

  it("names the line with a quotation mark out of place", () => {
    refused({
      field: "file",
      code: "unreadable",
      params: { reason: "quote", line: "3" },
    });
    expect(body()).toContain("Line 3 has a quotation mark out of place.");
  });

  it("says a file that could not be read at all, with no line", () => {
    refused({ field: "file", code: "unreadable" });
    expect(body()).toBe(en["import.refusal.unreadable"]);
  });

  it("names a column the header gives twice", () => {
    refused({
      field: "file",
      code: "header_unusable",
      params: { reason: "repeated_name", name: "email" },
    });
    expect(body()).toContain("The header row names “email” twice.");
  });

  it("names the header column with no name, and a file with no header", () => {
    refused({
      field: "file",
      code: "header_unusable",
      params: { reason: "blank_name", position: "4" },
    });
    expect(body()).toContain("Column 4 of the header row has no name.");
    cleanup();
    refused({
      field: "file",
      code: "header_unusable",
      params: { reason: "no_header" },
    });
    expect(body()).toContain("The file has no header row.");
  });

  it("names both files of one type in a ZIP, and the type", () => {
    refused({
      field: "file",
      code: "bundle_refused",
      params: {
        reason: "two_files_one_type",
        other_file: "export/contacts.csv",
        file: "CONTACTS.CSV",
        object: "contact",
      },
    });
    expect(body()).toBe(
      "“export/contacts.csv” and “CONTACTS.CSV” are both Contacts. Keep one of them and upload the ZIP again.",
    );
  });

  it("counts the files of a ZIP against the most an import reads", () => {
    refused({
      field: "file",
      code: "bundle_refused",
      params: { reason: "too_many_files", files: "21", max_files: "20" },
    });
    expect(body()).toContain(
      "The ZIP holds 21 files, and an import reads at most 20.",
    );
  });

  it("words every other reason a ZIP is refused", () => {
    const cases: [Record<string, string>, string][] = [
      [{ reason: "empty" }, "The ZIP is empty."],
      [{ reason: "not_a_zip" }, "The ZIP could not be opened."],
      [
        { reason: "files_too_large", max_mb: "1024" },
        "add up to more than 1,024 MB.",
      ],
      [{ reason: "zip_too_large", max_mb: "64" }, "larger than 64 MB"],
      [
        { reason: "password_protected", file: "deals.csv" },
        "“deals.csv” is password-protected.",
      ],
      [
        { reason: "file_damaged", file: "deals.csv" },
        "“deals.csv” in the ZIP is damaged.",
      ],
      [
        { reason: "file_missing", file: "deals.csv" },
        "“deals.csv” is not in the ZIP.",
      ],
      [
        { reason: "type_not_importable", object: "invoice" },
        "“invoice” is not a record type an import takes.",
      ],
      [
        { reason: "spreadsheet" },
        "This is an Excel workbook. Save the sheet as CSV UTF-8 and upload that.",
      ],
      [
        {
          reason: "no_importable_file",
          file_names: "companies.csv, deals.csv",
        },
        "Name each file after its record type: companies.csv, deals.csv.",
      ],
      [{ reason: "from_a_newer_server" }, "The ZIP cannot be imported."],
    ];
    for (const [params, sentence] of cases) {
      refused({ field: "file", code: "bundle_refused", params });
      expect(body()).toContain(sentence);
      cleanup();
    }
  });

  it("puts the count and the limit the server sent in a file too long", () => {
    refused({
      field: "file",
      code: "too_many_rows",
      max: 15000,
      params: { rows: "15001" },
    });
    expect(
      screen.getByText(en["import.upload.tooManyRowsTitle"]),
    ).toBeVisible();
    expect(body()).toBe(
      "This file has 15,001 rows, and one test run reads at most 15,000. Split it into smaller files and import each one.",
    );
  });

  it("says a file is too long without numbers when the server sent none", () => {
    refused({ field: "file", code: "too_many_rows" });
    expect(body()).toBe(en["import.upload.tooManyRows"]);
  });

  it("lists every type an import takes", () => {
    refused({ field: "object", code: "unsupported" });
    expect(body()).toBe(en["import.refusal.unsupported"]);
  });

  it("names two columns mapped to one field by the field's label", () => {
    refused(
      {
        field: "mapping",
        code: "duplicate_target",
        params: { target: "email", column: "alt_email", other_column: "email" },
      },
      formatWithLabels,
    );
    expect(body()).toBe(
      "The columns “email” and “alt_email” both map to “Email address”. Map one column to each field.",
    );
  });

  it("offers your own key beside the field that tells rows apart", () => {
    refused(
      {
        field: "source_key",
        code: "unmappable",
        params: { object: "contact", target: "email", ref_target: "ref" },
      },
      formatWithLabels,
    );
    expect(body()).toBe(
      "No column identifies each row. Map a column to “Email address” or to “Your own key (ref)”.",
    );
  });

  it("names only the key when the key is already your own", () => {
    refused(
      {
        field: "source_key",
        code: "unmappable",
        params: { object: "company", target: "ref" },
      },
      formatWithLabels,
    );
    expect(body()).toBe(
      "No column identifies each row. Map a column to “Your own key (ref)”.",
    );
  });

  it("states the source name rule with the limit the server sent", () => {
    refused({ field: "source_name", code: "invalid", max: 80 });
    expect(body()).toBe(
      "The source name is not allowed. Use up to 80 characters: letters A to Z, digits, spaces, dots, underscores and hyphens.",
    );
  });

  it("puts figures in the reader's number format", () => {
    refused(
      {
        field: "file",
        code: "unreadable",
        params: { reason: "field_count", line: "1204" },
      },
      undefined,
      "de",
    );
    expect(body()).toContain("Zeile 1.204 hat mehr oder weniger Felder");
  });

  it("words the map and file refusals that carry no values", () => {
    const cases: [Fault, string][] = [
      [{ field: "file", code: "empty" }, en["import.refusal.fileEmpty"]],
      [
        { field: "object", code: "mismatch" },
        en["import.refusal.objectMismatch"],
      ],
      [{ field: "mapping", code: "empty" }, en["import.refusal.mappingEmpty"]],
      [
        { field: "mapping", code: "unknown_column" },
        en["import.refusal.unknownColumn"],
      ],
      [
        { field: "mapping", code: "unknown_target" },
        en["import.refusal.unknownTarget"],
      ],
      [
        { field: "source_key", code: "unmapped_column" },
        en["import.refusal.unmappedColumn"],
      ],
    ];
    for (const [fault, sentence] of cases) {
      refused(fault);
      expect(body()).toBe(sentence);
      cleanup();
    }
  });

  it("keeps the server's sentence for a code this build does not know", () => {
    refused({ field: "file", code: "from_a_newer_server" }); // problem-code:unknown-field exercises an unfamiliar server refusal
    expect(body()).toBe(SERVER_SENTENCE);
  });
});

// A test run the worker refused stores the refusal's code beside its sentence.
describe("a run the background test refused", () => {
  const STORED = "Line 7 has more or fewer fields than the header row.";

  function StopLine({ run }: Readonly<{ run: ImportRun }>) {
    return <p data-testid="stop">{useRunStopSentence(run)}</p>;
  }

  function stopped(
    stored: Pick<ImportRun, "error" | "error_refusal">,
    locale: Locale,
  ): string {
    const run: ImportRun = {
      id: "r1",
      connector: "csv",
      status: "pending",
      checkpoint: 0,
      version: 1,
      source_released: false,
      source: "import_api",
      captured_by: "u-ada",
      created_at: "2026-10-12T09:30:00Z",
      updated_at: "2026-10-12T09:30:00Z",
      ...stored,
    };
    render(
      <StoryProviders locale={locale}>
        <StopLine run={run} />
      </StoryProviders>,
    );
    return screen.getByTestId("stop").textContent ?? "";
  }

  it("words its refusal in the reader's language", () => {
    const run = {
      error: STORED,
      error_refusal: {
        field: "file",
        code: "unreadable",
        params: { reason: "field_count", line: "7", file: "leads.csv" },
      },
    };
    expect(stopped(run, "de")).toBe(
      "In „leads.csv“: Zeile 7 hat mehr oder weniger Felder als die Kopfzeile. Korrigiere diese Zeile und lade die Datei erneut hoch.",
    );
  });

  it("shows the stored sentence of a run kept before its code was", () => {
    expect(stopped({ error: STORED }, "de")).toBe(STORED);
  });

  it("shows the stored sentence for a code this build does not know", () => {
    const run = {
      error: STORED,
      error_refusal: { field: "file", code: "from_a_newer_server" },
    };
    expect(stopped(run, "de")).toBe(STORED);
  });
});
