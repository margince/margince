// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { CompanyScreen } from "./companies";
import {
  company,
  companyBackstop,
  jsonResponse,
  stubFetch,
} from "./company.fixtures";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Dossier = components["schemas"]["CompanyDossier"];
type Receipt = components["schemas"]["ClaimEvidence"];

const DOSSIER: Dossier = {
  company_id: "o-1",
  generated_at: "2026-08-08T09:00:00Z",
  generated_by: "deterministic",
  sections: [
    {
      kind: "summary",
      sentences: [
        {
          text: "What they offer: load-shifting software.",
          nature: "fact",
          evidence: [
            { entity_type: "profile_field", entity_id: "p-1" },
            { entity_type: "profile_field", entity_id: "p-2" },
          ],
        },
      ],
    },
  ],
};

function receipt(id: string, value: string): Receipt {
  return {
    entity_type: "profile_field",
    entity_id: id,
    source_kind: "human",
    produced_by: "human:u1",
    value,
  };
}

const RECEIPTS: Record<string, Receipt> = {
  "p-1": receipt("p-1", "Load-shifting software"),
  "p-2": receipt("p-2", "Energy-intensive manufacturers"),
};

function drawAccount() {
  stubFetch(async (url) => {
    const asked = /\/evidence\/profile_field\/([^/?]+)/.exec(url);
    if (asked) {
      return jsonResponse(RECEIPTS[asked[1]]);
    }
    return url.includes("/dossier")
      ? jsonResponse(DOSSIER)
      : companyBackstop(url);
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>
          <CompanyScreen id="o-1" />
        </RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a cited profile field on the account overview", () => {
  it("opens its receipt in the page's drawer and steps to the sentence's next one", async () => {
    const user = userEvent.setup();
    drawAccount();
    await screen.findByRole("heading", { name: company.display_name });

    await user.click(
      await screen.findByRole("button", { name: "2 profile fields" }),
    );
    const drawer = await screen.findByRole("dialog", { name: "Source" });
    expect(
      await within(drawer).findByText("Load-shifting software"),
    ).toBeTruthy();

    await user.click(
      within(drawer).getByRole("button", { name: "Next claim" }),
    );
    expect(
      await within(drawer).findByText("Energy-intensive manufacturers"),
    ).toBeTruthy();
  });
});
