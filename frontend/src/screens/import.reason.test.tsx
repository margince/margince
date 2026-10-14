/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import type { Locale } from "../i18n/locale";
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

  it("asks the server for problems.csv in the reader's language", () => {
    expect(problemsHref("r 1", "vi")).toBe(
      "/v1/imports/r%201/problems.csv?lang=vi",
    );
  });
});
