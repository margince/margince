// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { FilingLine } from "./capture-filing";

afterEach(cleanup);

function show(filing: Parameters<typeof FilingLine>[0]["filing"]) {
  render(
    <LocaleProvider>
      <p data-testid="line">
        <FilingLine filing={filing} />
      </p>
    </LocaleProvider>,
  );
  return screen.getByTestId("line");
}

describe("the filing line", () => {
  it("names the rung when core filed the mail", () => {
    expect(
      show({ actor: "core", result: "linked", reason: "project" }),
    ).toHaveTextContent("Filed on its deal by the CRM: the mail’s project");
  });

  it("names the unit and falls back to its reason key without its copy", () => {
    expect(
      show({
        actor: "deal-rules",
        hook: "deal",
        result: "none",
        reason: "credit_hold",
      }),
    ).toHaveTextContent("deal-rules kept it off every deal: credit_hold");
  });

  it("says a failed hook left the mail to be tried again", () => {
    expect(
      show({ actor: "deal-rules", hook: "deal", result: "failed" }),
    ).toHaveTextContent("deal-rules failed, so the mail is left unfiled");
  });

  it("renders nothing for a result a newer binary invented", () => {
    expect(
      show({ actor: "deal-rules", result: "teleported" as "none" }),
    ).toBeEmptyDOMElement();
  });
});
