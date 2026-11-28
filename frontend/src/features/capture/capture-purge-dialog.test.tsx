// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
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
    screen.queryByRole("button", { name: englishCopy["capturePurge.confirm"] }),
  ).toBeNull();
  expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(2);
  expect(
    screen.getByRole("button", { name: "Check again" }),
  ).toBeInTheDocument();
});

const NONE = {
  held: 0,
  under_statute: 0,
  under_request: 0,
  under_undetermined_floor: 0,
};
const outcomeOf = (destroyed: number, preview: boolean) => ({
  preview,
  destroyed,
  released: 0,
  anonymised: 0,
  kept: NONE,
});
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });

function renderPurge() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <PurgeDialog ruleId="ex-2" ruleValue="example.test" onClose={vi.fn()} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Mail that arrives after a zero check is found by Check again, not left behind Close.
it("rechecks a zero count and then offers the delete for what arrived", async () => {
  const counts = [0, 2];
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => json(outcomeOf(counts.shift() ?? 2, true))),
  );
  const user = userEvent.setup();
  renderPurge();
  await user.click(screen.getByRole("button", { name: "Check first" }));
  await user.click(await screen.findByRole("button", { name: "Check again" }));

  expect(await screen.findByText(/2 messages would be/)).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: englishCopy["capturePurge.confirm"] }),
  ).toBeInTheDocument();
});

// The confirm carries the counts the reader saw. A 409 means more mail arrived,
// so the dialog shows the new count and asks again instead of deleting it unseen.
it("sends the previewed counts and re-checks when the server says they moved", async () => {
  const urls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      urls.push(request.url);
      if (request.url.includes("preview=true")) {
        return json(outcomeOf(urls.length === 1 ? 1 : 3, true));
      }
      return json({ code: "conflict", status: 409, title: "Conflict" }, 409);
    }),
  );
  const user = userEvent.setup();
  renderPurge();
  await user.click(screen.getByRole("button", { name: "Check first" }));
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["capturePurge.confirm"],
    }),
  );

  expect(await screen.findByText(/3 messages would be/)).toBeInTheDocument();
  expect(screen.getByText(/changed since the check/)).toBeInTheDocument();
  expect(urls[1]).toContain("expect_destroyed=1");
  expect(urls[1]).toContain("expect_released=0");
  expect(urls[1]).toContain("expect_anonymised=0");
  expect(screen.queryByText(/could not|failed/i)).toBeNull();
});
