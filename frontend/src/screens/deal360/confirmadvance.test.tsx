// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { holdExits } from "../../design-system/presence-testing";
import { LocaleProvider } from "../../i18n";
import { ConfirmAdvanceModal, type PendingAdvance } from "./confirmadvance";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const LOST: components["schemas"]["Stage"] = {
  id: "stage-lost",
  pipeline_id: "pl-1",
  name: "Closed lost",
  position: 4,
  semantic: "lost",
  win_probability: 0,
};

const WON: components["schemas"]["Stage"] = {
  ...LOST,
  id: "stage-won",
  name: "Closed won",
  semantic: "won",
  win_probability: 100,
};

it("keeps naming the stage it asked about while the dialog animates out", () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ data: [] })),
  );
  const exits = holdExits();
  const client = new QueryClient();
  const view = (pending: PendingAdvance | null) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConfirmAdvanceModal
          pending={pending}
          onClose={() => {}}
          onConfirm={async () => undefined}
        />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const { rerender } = render(
    view({ dealId: "deal-1", version: 3, toStage: LOST }),
  );
  const asked = screen.getByRole("heading").textContent;
  expect(asked).toContain("Closed lost");

  rerender(view(null));

  // A leaving dialog is aria-hidden, so the heading is read off the tree.
  expect(document.querySelector(".modal-title")?.textContent).toBe(asked);
  exits.mockRestore();
});

it("lets an advance abandoned mid-write leave the next one alone", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ data: [] })),
  );
  const user = userEvent.setup();
  const client = new QueryClient();
  let land: (deal: unknown) => void = () => {};
  const onClose = vi.fn();
  const onClosed = vi.fn();
  const view = (pending: PendingAdvance | null) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConfirmAdvanceModal
          pending={pending}
          onClose={onClose}
          onClosed={onClosed}
          onConfirm={() =>
            new Promise((resolve) => {
              land = resolve;
            })
          }
        />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const { rerender } = render(
    view({ dealId: "deal-a", version: 3, toStage: WON }),
  );
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await user.keyboard("{Escape}");
  expect(onClose).toHaveBeenCalledTimes(1);
  rerender(view(null));
  rerender(view({ dealId: "deal-b", version: 7, toStage: LOST }));

  expect(
    screen.getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled,
  ).toBe(false);
  await act(async () => land({ id: "deal-a", version: 4 }));

  expect(onClose).toHaveBeenCalledTimes(1);
  expect(onClosed).not.toHaveBeenCalled();
  expect(screen.getByRole("heading").textContent).toContain("Closed lost");
});
