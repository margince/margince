// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { ContractForm } from "./contractform";
import { ContractRenewModal } from "./contractlifecycle";

const CONTRACT: components["schemas"]["Contract"] = {
  id: "c-1",
  company_id: "o-1",
  title: "MSA 2026",
  status: "active",
  under_contract: true,
  auto_renew: false,
  value_basis: "total",
  starts_on: "2026-01-01",
  version: 1,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubApi() {
  const writes: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.method !== "GET") {
        writes.push(request);
      }
      return Response.json({ data: [] });
    }),
  );
  return writes;
}

function show(ui: ReactNode) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

async function typeUnreadableDay(
  user: ReturnType<typeof userEvent.setup>,
  box: HTMLElement,
) {
  await user.clear(box);
  await user.type(box, "31/02/2026");
}

it("refuses an edit whose start day cannot be read and keeps the stored day", async () => {
  const writes = stubApi();
  show(
    <ContractForm companyId="o-1" contract={CONTRACT} open onClose={vi.fn()} />,
  );
  const user = userEvent.setup();
  const box = await screen.findByLabelText(/^Starts/);
  await typeUnreadableDay(user, box);

  await user.click(screen.getByRole("button", { name: "Save changes" }));

  expect(writes).toHaveLength(0);
  expect(box).toHaveAttribute("aria-invalid", "true");
  expect((box as HTMLInputElement).validationMessage).toMatch(
    /^Enter a date as /,
  );
});

it("refuses a renewal whose start day cannot be read", async () => {
  const writes = stubApi();
  show(<ContractRenewModal contract={CONTRACT} open onClose={vi.fn()} />);
  const user = userEvent.setup();
  const box = await screen.findByLabelText(/^Starts/);
  await user.type(box, "01/01/2026");
  await typeUnreadableDay(user, box);

  await user.click(screen.getByRole("button", { name: "Renew" }));

  expect(writes).toHaveLength(0);
  expect((box as HTMLInputElement).validationMessage).toMatch(
    /^Enter a date as /,
  );
});
