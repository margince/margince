/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { CreateAction } from "./create";
import type { CreateField } from "./create.fields";

const fields: CreateField[] = [
  { key: "name", label: "create.dealName", required: true },
  { key: "expected_close_date", label: "create.expectedClose", type: "date" },
];

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 4, 20, 12));
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function renderNewDeal() {
  const create = vi.fn(async (_values: Record<string, string>) => ({
    id: "d-1",
  }));
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CreateAction
          label="New deal"
          fields={fields}
          create={create}
          invalidate="deals"
          screen="deals"
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { create };
}

describe("a date field in a create form", () => {
  it("sends the day picked in its calendar", async () => {
    const user = userEvent.setup();
    const { create } = renderNewDeal();
    await user.click(screen.getByText("New deal"));
    await user.type(screen.getByLabelText(/Deal name/i), "Probe");

    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    await user.click(
      await screen.findByRole("button", { name: "Friday, 22 May 2026" }),
    );
    expect(
      screen.getByRole<HTMLInputElement>("textbox", { name: "Expected close" })
        .value,
    ).toBe("22/05/2026");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
    expect(create.mock.calls[0]?.[0]).toMatchObject({
      expected_close_date: "2026-05-22",
    });
  });

  it("sends a day typed in the reader's pattern", async () => {
    const user = userEvent.setup();
    const { create } = renderNewDeal();
    await user.click(screen.getByText("New deal"));
    await user.type(screen.getByLabelText(/Deal name/i), "Probe");
    await user.type(
      screen.getByRole("textbox", { name: "Expected close" }),
      "3/6/2026",
    );
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
    expect(create.mock.calls[0]?.[0]).toMatchObject({
      expected_close_date: "2026-06-03",
    });
  });

  it("takes back a typed day on the first Escape and closes on the next", async () => {
    const user = userEvent.setup();
    renderNewDeal();
    await user.click(screen.getByText("New deal"));
    const close = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Expected close",
    });
    await user.type(close, "01/03/2027");

    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(close.value).toBe("");

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("closes only the calendar on an Escape inside it", async () => {
    const user = userEvent.setup();
    renderNewDeal();
    await user.click(screen.getByText("New deal"));
    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    const calendar = await screen.findByRole("dialog", {
      name: /^Choose date/,
    });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(calendar.isConnected).toBe(false));
    expect(
      screen.getByRole("textbox", { name: "Expected close" }),
    ).toBeTruthy();
  });
});
