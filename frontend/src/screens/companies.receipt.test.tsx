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
  company360,
  companyBackstop,
  jsonResponse,
} from "./company.fixtures";
import { stubFetch } from "./company.testkit";

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

function receipt(
  entityType: Receipt["entity_type"],
  id: string,
  value: string,
): Receipt {
  return {
    entity_type: entityType,
    entity_id: id,
    source_kind: "human",
    produced_by: "human:u1",
    value,
  };
}

const RECEIPTS: Record<string, Receipt> = {
  "p-1": receipt("profile_field", "p-1", "Load-shifting software"),
  "p-2": receipt("profile_field", "p-2", "Energy-intensive manufacturers"),
  "f-1": receipt("fact", "f-1", "240 employees"),
};

const HEADCOUNT = { entity_type: "fact", entity_id: "f-1" };

const ANSWER = {
  company_id: "o-1",
  question: "whats_open",
  generated_at: "2026-06-01T09:00:00Z",
  generated_by: "model",
  sentences: [
    { text: "They have grown past 200 staff.", evidence: [HEADCOUNT] },
  ],
};

const SUGGESTED = {
  ...company360,
  suggestions: [
    {
      kind: "no_reply",
      fingerprint: "s-1",
      reason: "They grew and nobody has written since.",
      evidence: [HEADCOUNT],
    },
  ],
};

function drawAccount(three60: unknown = company360) {
  stubFetch(
    async (url, method) => {
      const asked = /\/evidence\/(?:fact|profile_field)\/([^/?]+)/.exec(url);
      if (asked) {
        return jsonResponse(RECEIPTS[asked[1]]);
      }
      if (method === "POST" && url.includes("/ask")) {
        return jsonResponse(ANSWER);
      }
      return url.includes("/dossier")
        ? jsonResponse(DOSSIER)
        : companyBackstop(url);
    },
    { company360: three60 },
  );
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

async function openedReceipt(value: string) {
  const drawer = await screen.findByRole("dialog", { name: "Source" });
  expect(await within(drawer).findByText(value)).toBeTruthy();
  return drawer;
}

describe("a cited receipt on the company page opens the page's drawer", () => {
  it("from the overview's dossier, stepping to the sentence's next receipt", async () => {
    const user = userEvent.setup();
    drawAccount();
    await screen.findByRole("heading", { name: company.display_name });

    await user.click(
      await screen.findByRole("button", { name: "2 profile fields" }),
    );
    const drawer = await openedReceipt("Load-shifting software");
    await user.click(
      within(drawer).getByRole("button", { name: "Next claim" }),
    );
    expect(
      await within(drawer).findByText("Energy-intensive manufacturers"),
    ).toBeTruthy();
  });

  it("from the Profile tab's dossier", async () => {
    const user = userEvent.setup();
    drawAccount();
    await user.click(await screen.findByRole("button", { name: "Profile" }));

    await user.click(
      await screen.findByRole("button", { name: "2 profile fields" }),
    );
    await openedReceipt("Load-shifting software");
  });

  it("from a suggestion's grounds", async () => {
    const user = userEvent.setup();
    drawAccount(SUGGESTED);
    await user.click(
      await screen.findByText("They grew and nobody has written since."),
    );

    await user.click(await screen.findByRole("button", { name: "fact" }));
    await openedReceipt("240 employees");
  });

  it("from an answer to a prepared question", async () => {
    const user = userEvent.setup();
    drawAccount();
    await user.click(
      await screen.findByRole("button", { name: "What is open here?" }),
    );

    await user.click(await screen.findByRole("button", { name: "fact" }));
    await openedReceipt("240 employees");
  });
});
