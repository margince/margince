/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import type { Locale } from "../i18n/locale";
import { UndoErrors } from "./import.notices";
import { ProblemTable, problemsHref } from "./import.testrun";
import type { ImportRowIssue } from "./importtypes";

afterEach(cleanup);

function show(ui: ReactNode, locale: Locale) {
  return render(<LocaleProvider initial={locale}>{ui}</LocaleProvider>);
}

const coded: ImportRowIssue = {
  line: 3,
  file: "contacts.csv",
  kind: "broken_link",
  reason: 'no company with "CO-9" in this upload',
  reason_code: "link_not_found",
  params: { ref: "CO-9" },
};

describe("a problem's reason", () => {
  it("reads in the reader's language from its code and values", () => {
    show(<ProblemTable issues={[coded]} />, "de");
    expect(screen.getByText(/Kein Unternehmen mit „CO-9“/)).toBeVisible();
  });

  it("reads the sentence the server wrote when the report has no code", () => {
    const { reason_code: _code, params: _params, ...old } = coded;
    show(<ProblemTable issues={[old]} />, "de");
    expect(screen.getByText(old.reason)).toBeVisible();
  });

  it("reads the server's sentence when a value the code names is missing", () => {
    show(<ProblemTable issues={[{ ...coded, params: {} }]} />, "de");
    expect(screen.getByText(coded.reason)).toBeVisible();
  });

  it("reads the server's sentence, not the key, for a code this bundle does not know", () => {
    const unknown = {
      ...coded,
      reason_code: "a_newer_code",
    } as unknown as ImportRowIssue; // contract:cast a newer server sends a code this bundle lacks
    show(<ProblemTable issues={[unknown]} />, "de");
    expect(screen.getByText(coded.reason)).toBeVisible();
    expect(screen.queryByText(/import\.reason\./)).toBeNull();
  });

  it("names a type in the reader's language, not in the server's English", () => {
    const refused: ImportRowIssue = {
      ...coded,
      kind: "permission",
      reason: "You cannot create contacts, so the contacts file is skipped",
      reason_code: "cannot_create",
      params: { plural: "contacts" },
    };
    show(<ProblemTable issues={[refused]} />, "de");
    expect(screen.getByText(/vom Typ Kontakte anlegen/)).toBeVisible();
  });

  it("names the deals file by its type in the reader's language", () => {
    const refused: ImportRowIssue = {
      ...coded,
      kind: "permission",
      reason: "You cannot create deals, so the deals file is skipped",
      reason_code: "cannot_create",
      params: { plural: "deals" },
    };
    show(<ProblemTable issues={[refused]} />, "de");
    expect(screen.getByText(/vom Typ Deals anlegen/)).toBeVisible();
  });

  it("says an undo refusal from its code", () => {
    const row = {
      object: "contact" as const,
      id: "c-1",
      reason: "the record is no longer visible to this caller",
      reason_code: "undo_not_visible" as const,
    };
    show(<UndoErrors rows={[row]} />, "de");
    expect(screen.getByText(/nicht mehr sichtbar/)).toBeVisible();
    expect(screen.queryByText(row.reason)).toBeNull();
  });

  it("asks the server for problems.csv in the reader's language", () => {
    expect(problemsHref("r 1", "vi")).toBe(
      "/v1/imports/r%201/problems.csv?lang=vi",
    );
  });
});
