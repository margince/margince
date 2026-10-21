// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { holdExits } from "../../../design-system/presence-testing";
import { pickOption } from "../../../design-system/select-testing";
import { LocaleProvider } from "../../../i18n";
import { ProblemError } from "../../../lib/problem";
import {
  advanceErrorBehind,
  ConfirmAdvanceModal,
  type PendingAdvance,
} from "./confirmadvance";

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
    vi.fn(async () => Response.json({ data: [], page: { has_more: false } })),
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

it("holds the dialog through a write and closes it on the answer", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ data: [], page: { has_more: false } })),
  );
  const user = userEvent.setup();
  const client = new QueryClient();
  let land: (deal: unknown) => void = () => {};
  const onClose = vi.fn();
  const onClosed = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConfirmAdvanceModal
          pending={{ dealId: "deal-a", version: 3, toStage: WON }}
          onClose={onClose}
          onClosed={onClosed}
          onConfirm={() =>
            new Promise((resolve) => {
              land = resolve;
            })
          }
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  // Confirm waits for the deal's task read before it takes a press.
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Confirm" }).hasAttribute("disabled"),
    ).toBe(false),
  );
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await user.keyboard("{Escape}");
  expect(onClose).not.toHaveBeenCalled();
  expect(
    screen.getByRole("button", { name: "Confirm" }).getAttribute("aria-busy"),
  ).toBe("true");

  await act(async () => land({ id: "deal-a", version: 4 }));
  expect(onClosed).toHaveBeenCalledTimes(1);
  expect(onClose).toHaveBeenCalledTimes(1);
});

it("lets go of the dialog when the write rejects instead of holding it busy", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ data: [], page: { has_more: false } })),
  );
  const user = userEvent.setup();
  const client = new QueryClient();
  const onClose = vi.fn();
  let refuse: (error: Error) => void = () => {};
  const write = new Promise((_, reject) => {
    refuse = reject;
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConfirmAdvanceModal
          pending={{ dealId: "deal-a", version: 3, toStage: WON }}
          onClose={onClose}
          onConfirm={() => write}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  // Confirm waits for the deal's task read before it takes a press.
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Confirm" }).hasAttribute("disabled"),
    ).toBe(false),
  );
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await act(async () => {
    refuse(new Error("network down"));
    await write.catch(() => undefined);
  });
  expect(onClose).toHaveBeenCalledTimes(1);
  expect(
    screen.getByRole("button", { name: "Confirm" }).getAttribute("aria-busy"),
  ).toBeNull();
});

// What the deal's win-evidence read answers: the contract state, or a failure.
type Evidence = boolean | "unreadable";

// Answers the dialog's reads: the deal's open tasks (none) and its win
// evidence. A read held back by `hold` stays pending until it is released.
function stubReads(evidence: Evidence, hold?: Promise<void>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      if (!url.includes("/win-evidence")) {
        return Response.json({ data: [], page: { has_more: false } });
      }
      await hold;
      return evidence === "unreadable"
        ? Response.json({ code: "internal" }, { status: 500 })
        : Response.json({ has_signed_contract: evidence });
    }),
  );
}

function openOn(
  toStage: components["schemas"]["Stage"],
  onConfirm: (input: unknown) => Promise<unknown>,
  evidence: Evidence = true,
) {
  stubReads(evidence);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <ConfirmAdvanceModal
          pending={{ dealId: "deal-a", version: 3, toStage }}
          onClose={() => {}}
          onConfirm={onConfirm}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The sentence a refused Confirm points at, or null when it names none.
function confirmRefusal(): string | null {
  const ids = screen
    .getByRole("button", { name: "Confirm" })
    .getAttribute("aria-describedby");
  const said = (ids ?? "")
    .split(" ")
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ")
    .trim();
  return said === "" ? null : said;
}

it("says a lost deal needs a reason, and sends the reason on Enter", async () => {
  const user = userEvent.setup();
  const onConfirm = vi.fn(async () => undefined);
  openOn(LOST, onConfirm);

  await waitFor(() =>
    expect(confirmRefusal()).toBe("Enter why the deal was lost."),
  );
  await user.click(screen.getByRole("textbox", { name: "Lost reason" }));
  await user.keyboard("{Enter}");
  expect(onConfirm).not.toHaveBeenCalled();

  await user.keyboard("Budget cut{Enter}");
  expect(onConfirm).toHaveBeenCalledTimes(1);
  expect(onConfirm).toHaveBeenCalledWith(
    expect.objectContaining({ lostReason: "Budget cut" }),
  );
});

const WIN_EVIDENCE_REFUSAL = new ProblemError({
  code: "validation_error",
  details: {
    errors: [
      {
        field: "won_without_contract_reason",
        code: "win_evidence_required",
        message: "a won deal needs a signed contract",
      },
    ],
  },
});

// An open dialog asks how the deal was won, so the screen behind it stays
// quiet. Once the reader cancels, the screen says why the deal did not move.
it("leaves the win-evidence refusal to the open dialog and shows it once closed", () => {
  expect(advanceErrorBehind(WIN_EVIDENCE_REFUSAL, true)).toBeNull();
  expect(advanceErrorBehind(WIN_EVIDENCE_REFUSAL, false)).toBe(
    WIN_EVIDENCE_REFUSAL,
  );
  const conflict = new ProblemError({ code: "version_skew" });
  expect(advanceErrorBehind(conflict, true)).toBe(conflict);
});

it("says a win marked Other needs details, and sends them on Enter", async () => {
  const user = userEvent.setup();
  const onConfirm = vi
    .fn<(input: unknown) => Promise<unknown>>()
    .mockRejectedValueOnce(WIN_EVIDENCE_REFUSAL)
    .mockResolvedValue(undefined);
  openOn(WON, onConfirm);
  await waitFor(() => expect(confirmRefusal()).toBeNull());
  await user.click(screen.getByRole("button", { name: "Confirm" }));

  const how = await screen.findByRole("combobox", { name: "How was it won?" });
  expect(confirmRefusal()).toBe("Pick how the deal was won.");
  await pickOption(user, how, "Other");
  expect(confirmRefusal()).toBe("Describe how the deal was won.");

  await user.click(screen.getByRole("textbox", { name: "Details" }));
  await user.keyboard("Signed on a napkin{Enter}");
  expect(onConfirm).toHaveBeenCalledTimes(2);
  expect(onConfirm).toHaveBeenLastCalledWith(
    expect.objectContaining({
      wonWithoutContractReason: "other",
      wonWithoutContractDetail: "Signed on a napkin",
    }),
  );
});

it("asks how a win happened before Confirm when the deal has no signed contract", async () => {
  const user = userEvent.setup();
  const onConfirm = vi.fn(async () => undefined);
  openOn(WON, onConfirm, false);

  const how = await screen.findByRole("combobox", { name: "How was it won?" });
  expect(confirmRefusal()).toBe("Pick how the deal was won.");
  await pickOption(user, how, "On a purchase order");
  expect(confirmRefusal()).toBeNull();

  await user.click(screen.getByRole("button", { name: "Confirm" }));
  expect(onConfirm).toHaveBeenCalledTimes(1);
  expect(onConfirm).toHaveBeenCalledWith(
    expect.objectContaining({ wonWithoutContractReason: "purchase_order" }),
  );
});

it("drops the answer when a contract was filed while the dialog sat open", async () => {
  const user = userEvent.setup();
  const onConfirm = vi.fn(async () => undefined);
  openOn(WON, onConfirm, false);

  const how = await screen.findByRole("combobox", { name: "How was it won?" });
  await pickOption(user, how, "Verbally, in person or by phone");
  stubReads(true);
  await user.click(screen.getByRole("button", { name: "Confirm" }));

  await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  expect(onConfirm).toHaveBeenCalledWith(
    expect.not.objectContaining({
      wonWithoutContractReason: expect.anything(),
    }),
  );
});

it("keeps a win with a signed contract to one click with nothing asked", async () => {
  const user = userEvent.setup();
  const onConfirm = vi.fn(async () => undefined);
  openOn(WON, onConfirm, true);

  await waitFor(() => expect(confirmRefusal()).toBeNull());
  expect(
    screen.queryByRole("combobox", { name: "How was it won?" }),
  ).toBeNull();
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  expect(onConfirm).toHaveBeenCalledTimes(1);
  expect(onConfirm).toHaveBeenCalledWith(
    expect.not.objectContaining({
      wonWithoutContractReason: expect.anything(),
    }),
  );
});

it("holds Confirm while it checks whether the deal has a signed contract", async () => {
  let release: () => void = () => {};
  stubReads(
    false,
    new Promise<void>((resolve) => {
      release = resolve;
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <ConfirmAdvanceModal
          pending={{ dealId: "deal-a", version: 3, toStage: WON }}
          onClose={() => {}}
          onConfirm={async () => undefined}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await waitFor(() =>
    expect(confirmRefusal()).toBe("Checking the deal’s contract…"),
  );
  expect(
    screen.queryByRole("combobox", { name: "How was it won?" }),
  ).toBeNull();

  await act(async () => release());
  await screen.findByRole("combobox", { name: "How was it won?" });
});

it("falls back to asking after a refusal when the contract check cannot be read", async () => {
  const user = userEvent.setup();
  const onConfirm = vi
    .fn<(input: unknown) => Promise<unknown>>()
    .mockRejectedValueOnce(WIN_EVIDENCE_REFUSAL)
    .mockResolvedValue(undefined);
  openOn(WON, onConfirm, "unreadable");

  await waitFor(() => expect(confirmRefusal()).toBeNull());
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await screen.findByRole("combobox", { name: "How was it won?" });
});

it("does not look for a contract when the deal is going to a lost stage", async () => {
  openOn(
    LOST,
    vi.fn(async () => undefined),
    false,
  );
  await waitFor(() =>
    expect(confirmRefusal()).toBe("Enter why the deal was lost."),
  );
  expect(
    screen.queryByRole("combobox", { name: "How was it won?" }),
  ).toBeNull();
  const reads = vi
    .mocked(fetch)
    .mock.calls.map(([input]) =>
      input instanceof Request ? input.url : String(input),
    );
  expect(reads.some((url) => url.includes("/win-evidence"))).toBe(false);
});
