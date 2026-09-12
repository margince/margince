/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { DealBrief } from "./dealbrief";

// Two defects this panel had. It VANISHED on a deal with no brief, so the
// field looked like one the product does not have — the reason a reviewer
// opening an ordinary deal concluded the work was missing. And it offered no
// way to write the brief it existed to show.

const DEAL_ID = "11111111-1111-1111-1111-111111111111";

type Sent = { body: unknown; ifMatch: string | null };

function stubFetch(sent: Sent[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.method === "PATCH") {
        sent.push({
          body: await req.clone().json(),
          ifMatch: req.headers.get("If-Match"),
        });
      }
      return new Response(JSON.stringify({ id: DEAL_ID, version: 3 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function render(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return rtlRender(
    <QueryClientProvider client={qc}>
      <LocaleProvider>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("names the field on a deal that has no brief", async () => {
  stubFetch([]);
  render(<DealBrief dealId={DEAL_ID} version={2} brief={null} />);
  // It used to render nothing at all here, so a reader could not tell the
  // brief from a field the product does not have.
  expect(screen.getByText(/No brief written yet/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Write the brief" })).toBeTruthy();
});

it("offers no way in on a deal this reader cannot write", () => {
  stubFetch([]);
  render(<DealBrief dealId={DEAL_ID} version={2} brief={null} readOnly />);
  // The panel still says the field exists; only the verb goes.
  expect(screen.getByText(/No brief written yet/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Write the brief" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
});

it("saves the brief alone, pinned to the version it read", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  render(<DealBrief dealId={DEAL_ID} version={2} brief={null} />);
  await userEvent.click(
    screen.getByRole("button", { name: "Write the brief" }),
  );
  await userEvent.type(screen.getByRole("textbox"), "Three warehouses.");
  await userEvent.click(screen.getByRole("button", { name: "Save brief" }));
  // One field, not the whole record: a PATCH naming every value would send
  // back what this modal never showed the reader and overwrite with it.
  expect(sent).toHaveLength(1);
  expect(sent[0].body).toEqual({ description: "Three warehouses." });
  // Unpinned is last-write-wins, which for prose two people might both be
  // rewriting is the loss worth refusing.
  expect(sent[0].ifMatch).toBe("2");
});

it("treats clearing the brief as a real edit", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  render(<DealBrief dealId={DEAL_ID} version={2} brief="Old words." />);
  await userEvent.click(screen.getByRole("button", { name: "Edit" }));
  await userEvent.clear(screen.getByRole("textbox"));
  await userEvent.click(screen.getByRole("button", { name: "Save brief" }));
  // null, not "" and not a refusal: deleting a brief that is wrong is
  // something a reader is entitled to do.
  expect(sent[0].body).toEqual({ description: null });
});

it("sends nothing when the brief was not changed", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  render(<DealBrief dealId={DEAL_ID} version={2} brief="Old words." />);
  await userEvent.click(screen.getByRole("button", { name: "Edit" }));
  // An unchanged box is not an edit. Saving it would bump the version under
  // everybody else for nothing.
  expect(
    screen.getByRole("button", { name: "Save brief" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(sent).toHaveLength(0);
});

it("sends the brief as typed, whitespace and all", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  render(<DealBrief dealId={DEAL_ID} version={2} brief="Old words." />);
  await userEvent.click(screen.getByRole("button", { name: "Edit" }));
  const box = screen.getByRole("textbox");
  await userEvent.clear(box);
  await userEvent.type(box, "  Padded on purpose.  ");
  await userEvent.click(screen.getByRole("button", { name: "Save brief" }));
  // The server stores the description verbatim. Trimming here would silently
  // drop spacing a reader wrote, and would make a whitespace-only correction
  // impossible to save at all.
  expect(sent[0].body).toEqual({ description: "  Padded on purpose.  " });
});

it("keeps an unsaved draft when the deal refetches underneath it", async () => {
  const sent: Sent[] = [];
  stubFetch(sent);
  const { rerender } = render(
    <DealBrief dealId={DEAL_ID} version={2} brief="Old words." />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Edit" }));
  const box = screen.getByRole("textbox") as HTMLTextAreaElement;
  await userEvent.clear(box);
  await userEvent.type(box, "Half a sentence");
  // A colleague saves elsewhere and the background refetch lands. The modal is
  // OPEN and holds an unsaved draft: re-seeding here would replace a reader's
  // words with somebody else's, mid-sentence, with no warning.
  rerender(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider>
        <DealBrief dealId={DEAL_ID} version={3} brief="Their words." />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(box.value).toBe("Half a sentence");
});
