/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ReadCompanyStep } from "./onboarding-read";

type CompanySiteRead = components["schemas"]["CompanySiteRead"];

function fields(count: number): components["schemas"]["ColdStartField"][] {
  return Array.from({ length: count }, () => ({
    field: "industry",
    value: "Manufacturing",
    evidence_snippet: "We serve manufacturers.",
    source_kind: "url",
    source_url: "https://gradion.com",
    confidence: 0.9,
  }));
}

function read(
  status: CompanySiteRead["status"],
  findings: number,
): CompanySiteRead {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    target_kind: "onboarding",
    company_id: null,
    root_url: "https://gradion.com",
    status,
    status_code: null,
    status_detail: null,
    next_attempt_at: null,
    phase: null,
    pages_read: 1,
    pages: [{ url: "https://gradion.com", status: "fetched", kind: "home" }],
    profile_fields: fields(findings),
    facts: [],
    comparisons: [],
    contacts: [],
    warnings: [],
    draft_version: 1,
    proposal_hash: "proposal-1",
    created_at: "2026-07-22T08:00:00Z",
    updated_at: "2026-07-22T08:00:01Z",
  };
}

const noAction = () => undefined;

function renderStep(siteRead: CompanySiteRead) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ReadCompanyStep
          mode="website"
          website="gradion.com"
          norm={{ ok: true, host: "gradion.com", full: "https://gradion.com" }}
          read={siteRead}
          pending={false}
          refreshing={false}
          error={null}
          companyDraft={{}}
          confirmPending={false}
          confirmDisabled={false}
          onWebsiteChange={noAction}
          onChooseManual={noAction}
          onStart={noAction}
          onConfirm={noAction}
          onApplyChanges={noAction}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the finished read's heading", () => {
  it.each([
    ["ready", 1, "1 cited company detail found"],
    ["ready", 2, "2 cited company details found"],
    ["partial", 1, "1 useful detail found. Some gaps remain."],
    ["partial", 2, "2 useful details found. Some gaps remain."],
  ] as const)("names a %s read of %i finding(s)", (status, count, heading) => {
    renderStep(read(status, count));
    expect(screen.getByRole("heading", { name: heading })).toBeInTheDocument();
  });
});
