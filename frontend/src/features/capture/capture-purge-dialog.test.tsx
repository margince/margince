// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
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
            detail: "your role does not grant delete on activity",
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

// A check that finds nothing leaves no act to confirm, so no red button.
it("offers only Close when the check counts nothing to delete", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            preview: true,
            destroyed: 0,
            released: 0,
            anonymised: 0,
            kept: {
              held: 0,
              under_statute: 0,
              under_request: 0,
              under_undetermined_floor: 0,
            },
          }),
          { headers: { "content-type": "application/json" } },
        ),
    ),
  );
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <PurgeDialog ruleId="ex-2" ruleValue="example.test" onClose={vi.fn()} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "Check first" }));

  expect(await screen.findByText(/0 messages would be/)).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Delete permanently" }),
  ).toBeNull();
  expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(2);
});
