/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { RefreshModelPrices } from "./rate-catalogue-refresh";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const REPORT = {
  providers: [
    {
      provider: "openai_compatible",
      outcome: "updated",
      updated: 2,
      unchanged: 1,
      models: ["a/b", "c/d"],
    },
    {
      provider: "gemini",
      outcome: "not_available",
      updated: 0,
      unchanged: 0,
      models: [],
    },
    {
      provider: "jev_compatible",
      outcome: "unreachable",
      updated: 0,
      unchanged: 0,
      models: [],
    },
  ],
};

// Returns the fetch mock so a test can read exactly what the click sent.
function mount(respond: () => Response) {
  const fetchMock = vi.fn(async (_request: Request) => respond());
  vi.stubGlobal("fetch", fetchMock);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  render(
    <QueryClientProvider client={qc}>
      <LocaleProvider>
        <RefreshModelPrices />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { fetchMock, invalidate };
}

describe("RefreshModelPrices", () => {
  it("posts the refresh and lists one line per provider", async () => {
    const user = userEvent.setup();
    const { fetchMock } = mount(() => jsonResponse(REPORT));
    expect(screen.queryByRole("list")).toBeNull();

    await user.click(
      screen.getByRole("button", { name: "Refresh model prices" }),
    );

    const report = await screen.findByRole("list", {
      name: "Model price refresh",
    });
    const lines = within(report).getAllByRole("listitem");
    expect(lines).toHaveLength(3);
    expect(within(lines[0]).getByText("Updated")).toBeTruthy();
    expect(within(lines[0]).getByText("openai_compatible")).toBeTruthy();
    expect(within(lines[0]).getByText("2 prices written")).toBeTruthy();
    expect(within(lines[0]).getByText("1 price already current")).toBeTruthy();
    // A vendor with no price list says so and counts nothing.
    expect(within(lines[1]).getByText("Set by hand")).toBeTruthy();
    expect(within(lines[1]).queryByText(/prices? written/)).toBeNull();
    expect(within(lines[2]).getByText("Unreachable")).toBeTruthy();

    const request = fetchMock.mock.calls[0]?.[0];
    expect(request?.method).toBe("POST");
    expect(new URL(request?.url ?? "").pathname).toBe(
      "/v1/ai-model-rates/refresh",
    );
  });

  it("refreshes the lane prices once the sheet is written", async () => {
    const user = userEvent.setup();
    const { invalidate } = mount(() => jsonResponse(REPORT));
    await user.click(
      screen.getByRole("button", { name: "Refresh model prices" }),
    );
    await screen.findByRole("list");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["ai-model-rates"] });
  });

  it("speaks a refusal beside the button and keeps it pressable", async () => {
    const user = userEvent.setup();
    mount(() =>
      jsonResponse(
        {
          type: "https://errors.gradion.com/forbidden",
          title: "Forbidden",
          status: 403,
          code: "permission_denied",
          detail: "Refreshing prices needs the rates grant.",
        },
        403,
      ),
    );
    await user.click(
      screen.getByRole("button", { name: "Refresh model prices" }),
    );
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.queryByRole("list")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Refresh model prices" }),
    ).toHaveProperty("disabled", false);
  });
});
