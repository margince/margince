// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
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
