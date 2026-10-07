// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { PurgeDialog } from "./capture-purge-dialog";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("holds every way out while the purge is in flight", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  const user = userEvent.setup();
  const onClose = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <PurgeDialog
          ruleId="ex-2"
          ruleValue="recruiting.example"
          onClose={onClose}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "Check first" }));

  expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
  await user.keyboard("{Escape}");
  expect(onClose).not.toHaveBeenCalled();
});

// A refusal before anything ran destroyed nothing, and the dialog must not tell
// the reader it may have.
it("says nothing was destroyed when the purge is refused", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            title: "Forbidden",
            status: 403,
            code: "permission_denied",
            detail: "activity.delete: permission denied",
          }),
          {
            status: 403,
            headers: { "content-type": "application/problem+json" },
          },
        ),
    ),
  );
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <PurgeDialog
          ruleId="ex-2"
          ruleValue="recruiting.example"
          onClose={vi.fn()}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "Check first" }));

  expect(await screen.findByText(/nothing was destroyed/i)).toBeInTheDocument();
  expect(screen.queryByText(/may already have been destroyed/i)).toBeNull();
});
