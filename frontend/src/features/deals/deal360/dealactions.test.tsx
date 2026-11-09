/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { RecordShell } from "../../../testing/recordshell.testkit";
import { stubWithSession } from "../../../testing/story-utils";
import { DealActions } from "./dealactions";

type Deal = components["schemas"]["Deal"];

const deal = {
  id: "d-1",
  name: "Fleet telematics rollout",
  amount_minor: 4_500_000,
  currency: "EUR",
  stage_id: "st-1",
  status: "open",
  stalled: false,
  source: "manual",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
} as Deal;

afterEach(cleanup);

function mount(allow: Parameters<typeof stubWithSession>[1]) {
  stubWithSession({}, allow);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <RecordShell>
          <DealActions
            deal={deal}
            openStages={[]}
            onEditDetails={() => undefined}
          />
        </RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the deal header's Email verb", () => {
  it("is refused without the activity grant, over the same sentence as Log", async () => {
    mount({ deal: ["read"] });

    const log = await screen.findByRole("button", { name: "Log activity" });
    const email = screen.getByRole("button", { name: "Email" });
    await expect
      .poll(() => email.getAttribute("aria-describedby"))
      .toBeTruthy();
    expect(email.hasAttribute("disabled")).toBe(true);
    expect(email.getAttribute("aria-describedby")).toBe(
      log.getAttribute("aria-describedby"),
    );
    expect(
      document.getElementById(email.getAttribute("aria-describedby") ?? "")
        ?.textContent,
    ).toBe("You do not have permission to log activities on this record.");
  });

  it("is live with the activity grant", async () => {
    mount({ deal: ["read"], activity: ["create"] });

    const email = await screen.findByRole("button", { name: "Email" });
    await expect.poll(() => email.hasAttribute("disabled")).toBe(false);
  });
});
