// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { DraftContentNotice } from "./compose.notices";

afterEach(cleanup);

it("names the claim that needs a proof and the language the mail must open in", () => {
  render(
    <LocaleProvider initial="en">
      <DraftContentNotice
        findings={[
          {
            requirement: "claim_without_proof",
            detail: "the draft claims",
            words: ["tốt nhất"],
          },
          {
            requirement: "advertising_language_first",
            detail: "the draft does not open in vi",
            language: "vi",
          },
        ]}
      />
    </LocaleProvider>,
  );
  expect(screen.getByText(/“tốt nhất” is a claim/)).toBeInTheDocument();
  expect(
    screen.getByText(/Write this mail in Vietnamese first/),
  ).toBeInTheDocument();
});

it("tells a sender from abroad to advertise through a local business", () => {
  render(
    <LocaleProvider initial="en">
      <DraftContentNotice
        findings={[
          {
            requirement: "foreign_advertiser",
            detail: "the sending entity is not established in vn",
            country: "vn",
          },
        ]}
      />
    </LocaleProvider>,
  );
  expect(
    screen.getByText(/Your company is not set up in Vietnam\./),
  ).toBeInTheDocument();
});

it("says nothing when the draft meets every rule", () => {
  const { container } = render(
    <LocaleProvider initial="en">
      <DraftContentNotice findings={[]} />
    </LocaleProvider>,
  );
  expect(container).toBeEmptyDOMElement();
});
