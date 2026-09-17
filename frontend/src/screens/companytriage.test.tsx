/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { CompanyTriageSection } from "./companytriage";
import { jsonResponse, render } from "./settings.testkit";

const COMPANY = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e14";

function stub(body: unknown, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(body, status)),
  );
}

// The section is closed by default — this answers a question a reader asks
// once — so every case opens it before reading the body.
async function openSection() {
  // The summary element, not the title inside it: SectionSummary renders the
  // heading and the <summary> wraps it, so a text match finds both.
  await userEvent.click(
    screen.getByRole("group").querySelector("summary") as Element,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("names each checked domain and what its check concluded", async () => {
  stub({
    company_id: COMPANY,
    domains: [
      {
        domain: "nordwind.example",
        rung: {
          stage: "company_triage",
          order: 90,
          subject_kind: "domain",
          status: "done",
          reason: "company_warranted",
          reason_text: "the server's own sentence",
          at: "2026-09-17T09:00:00Z",
        },
      },
    ],
  });

  render(<CompanyTriageSection companyId={COMPANY} />);
  await openSection();

  expect(await screen.findByText("nordwind.example")).toBeInTheDocument();
  // This build's own catalog, not the server's sentence: both are true, and
  // the catalog is the one that is translated.
  expect(
    await screen.findByText(/judged to warrant a company record/),
  ).toBeInTheDocument();
  expect(
    screen.queryByText(/the server's own sentence/),
  ).not.toBeInTheDocument();
});

it("says no domain was checked in rather than showing an empty list", async () => {
  stub({ company_id: COMPANY, domains: [] });

  render(<CompanyTriageSection companyId={COMPANY} />);
  await openSection();

  // A company nobody triaged into was typed in or imported, which is ordinary
  // — and a blank body would leave a reader unable to tell that from a read
  // that failed.
  expect(
    await screen.findByText(/recorded by hand or brought in from elsewhere/),
  ).toBeInTheDocument();
});

it("reports a failed read as unavailable rather than as nothing checked", async () => {
  stub({ title: "no", status: 503 }, 503);

  render(<CompanyTriageSection companyId={COMPANY} />);
  await openSection();

  // The distinction the whole section turns on: "no domain was checked" is a
  // claim about how this record came to exist, and a request that did not
  // answer cannot support it.
  expect(
    screen.queryByText(/recorded by hand or brought in from elsewhere/),
  ).not.toBeInTheDocument();
});
