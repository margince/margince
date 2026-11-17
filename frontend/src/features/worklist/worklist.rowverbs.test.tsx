/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { overnightRow } from "../../testing/brief.fixtures";
import type { WorklistItem } from "./worklist.queries";
import { RowActs } from "./worklist.rowverbs";

const RECEIPT = {
  deal_id: "deal-expansion",
  rule: "stalled",
  policy_version: "v1",
  basis_fingerprint: "fp-1",
  checked_at: "2026-11-02T08:00:00Z",
};

// A deal's move the server stands behind, checked again on the press.
const row: WorklistItem = {
  ...overnightRow("move-1", RECEIPT.deal_id),
  move: { action: "draft_email", recommendation: RECEIPT },
};

let answer: () => void = () => undefined;
const status = vi.fn();

beforeEach(() => {
  globalThis.history.replaceState(null, "", "#/home?queue=1");
  status.mockReset();
  vi.stubGlobal(
    "fetch",
    (request: Request) =>
      new Promise<Response>((resolve) => {
        status(new URL(request.url).pathname);
        answer = () =>
          resolve(Response.json({ next: { recommendation: RECEIPT } }));
      }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function pressMove() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <RowActs item={row} title="Expansion" href={undefined} owner="" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return userEvent
    .setup()
    .click(screen.getByRole("link", { name: en["worklist.verb.draft_email"] }));
}

it("opens the move's record once the server still stands behind it", async () => {
  const entries = globalThis.history.length;
  await pressMove();
  await waitFor(() =>
    expect(status).toHaveBeenCalledWith(`/v1/deals/${RECEIPT.deal_id}/status`),
  );
  answer();
  await waitFor(() =>
    expect(globalThis.location.hash).toBe(`#/deals/${RECEIPT.deal_id}`),
  );
  expect(globalThis.history.length).toBe(entries + 1);
});

it("leaves a reader who moved on during the check where they are", async () => {
  await pressMove();
  await waitFor(() => expect(status).toHaveBeenCalled());
  globalThis.history.replaceState(null, "", "#/contacts");
  answer();
  const link = screen.getByRole("link", {
    name: en["worklist.verb.draft_email"],
  });
  await waitFor(() => expect(link).toHaveAttribute("aria-busy", "false"));
  expect(globalThis.location.hash).toBe("#/contacts");
});
