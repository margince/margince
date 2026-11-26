/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { NewDealAction } from "./companyactions";

// A company's New deal reads the amount the German way: one comma is the
// decimal sign, and "1.500" is refused because it also reads as 1500.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function createWithAmount(amount: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  const me = meFixture({ allow: { deal: ["create"], company: ["read"] } });
  client.setQueryData(["me"], me);
  client.setQueryData(
    ["pipelines", "all"],
    [
      {
        id: "pipeline-1",
        name: "Sales",
        is_default: true,
        position: 0,
        stages: [
          {
            id: "stage-1",
            pipeline_id: "pipeline-1",
            name: "New",
            semantic: "open",
            position: 0,
            win_probability: 10,
          },
        ],
      },
    ],
  );
  const sent: unknown[] = [];
  installFetchStub({
    "POST /deals": (body) => {
      sent.push(body);
      return jsonResponse({ id: "deal-1", name: "New deal" });
    },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="de">
        <NewDealAction
          companyId="company-1"
          companyName="Firma"
          onCreated={() => {}}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Neuer Deal" }));
  await user.type(
    screen.getByRole("textbox", { name: /^Deal-Name/ }),
    "Rollout",
  );
  await user.type(screen.getByRole("textbox", { name: /^Wert/ }), amount);
  await user.click(screen.getByRole("button", { name: "Erstellen" }));
  return sent;
}

it("sends 12,5 as 1250 minor units", async () => {
  const sent = await createWithAmount("12,5");
  await waitFor(() => expect(sent).toHaveLength(1));
  expect(sent[0]).toMatchObject({ amount_minor: 1250 });
});

it("refuses 1.500 by naming the wanted form, and sends nothing", async () => {
  const sent = await createWithAmount("1.500");
  expect(
    await screen.findByText("Gib den Betrag als 1500 oder 1500,00 ein."),
  ).toBeTruthy();
  expect(sent).toEqual([]);
});
