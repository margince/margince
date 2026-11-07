// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { QuickCaptureAction } from "./quickcapture";

const emptyPage = { data: [], page: { next_cursor: null, has_more: false } };

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubApi() {
  const posted: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (url.pathname.endsWith("/contacts/quick-capture")) {
        posted.push(JSON.parse(await request.text()));
        return json({ contact: { id: "p-new", full_name: "Dana Buyer" } }, 201);
      }
      if (url.pathname.endsWith("/me")) {
        return json(meFixture({ allow: { contact: ["read", "create"] } }));
      }
      return json(emptyPage);
    }),
  );
  return posted;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Quick capture", () => {
  it("marks the companies list stale once a typed company is saved", async () => {
    const posted = stubApi();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const companiesList = ["companies", {}];
    client.setQueryData(companiesList, emptyPage);
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ToastProvider>
            <QuickCaptureAction />
            <ToastRegion />
          </ToastProvider>
        </LocaleProvider>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();

    await user.click(screen.getByTestId("quick-capture"));
    await user.type(await screen.findByLabelText("Full name *"), "Dana Buyer");
    await user.type(
      screen.getByRole("combobox", { name: "Company" }),
      "Brandt Automotive GmbH",
    );
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(await screen.findByText("Dana Buyer saved")).toBeInTheDocument();
    expect(posted[0]).toMatchObject({ company_name: "Brandt Automotive GmbH" });
    await waitFor(() =>
      expect(client.getQueryState(companiesList)?.isInvalidated).toBe(true),
    );
  });
});
