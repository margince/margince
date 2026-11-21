/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/story-utils";
import { OfferScreen } from "./offers";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const baseLine = {
  id: "li-1",
  position: 1,
  product_id: null,
  description: "Consulting",
  unit: "hour",
  quantity: 10,
  unit_price_minor: 10000,
  discount_pct: 0,
  tax_rate: 19,
  line_net_minor: 100000,
  line_tax_minor: 19000,
  line_total_minor: 119000,
  evidence: null,
  price_grounded: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const offer = {
  id: "o-1",
  deal_id: "d-1",
  offer_number: "ANG-2026-0007",
  revision: 1,
  status: "draft",
  currency: "EUR",
  buyer_company_id: null,
  valid_until: "2026-08-01",
  intro_text: null,
  terms_text: null,
  net_minor: 100000,
  tax_minor: 19000,
  gross_minor: 119000,
  template_id: null,
  line_items: [baseLine],
  source: "manual",
  captured_by: "human:u1",
  version: 3,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const REFUSAL = "the server does not take this value";

type Held = {
  patch: Record<string, unknown>;
  answer: (status?: number) => void;
};

// A server that keeps the line as the PATCHes wrote it. Each PATCH waits for
// the test to answer it. The line PATCH carries no version, so the last one
// to land wins; the stub counts how many were ever in flight together.
function stubServer(initial: Record<string, unknown>) {
  const held: Held[] = [];
  const state = { line: initial, inflight: 0, mostInflight: 0 };
  const current = () => ({ ...offer, line_items: [state.line] });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request ? request.method : (init?.method ?? "GET");
      if (url.includes("/line-items/") && method === "PATCH") {
        const patch = request
          ? await request.json()
          : JSON.parse(String(init?.body));
        state.inflight += 1;
        state.mostInflight = Math.max(state.mostInflight, state.inflight);
        const status = await new Promise<number>((answer) =>
          held.push({ patch, answer: (code = 200) => answer(code) }),
        );
        state.inflight -= 1;
        if (status !== 200) {
          return jsonResponse(
            { title: "Unprocessable", status, detail: REFUSAL },
            status,
          );
        }
        state.line = { ...state.line, ...patch, version: 2 };
        return jsonResponse(current());
      }
      if (url.includes("/offers/")) {
        return jsonResponse(current());
      }
      return jsonResponse({ data: [], page: { has_more: false } });
    }),
  );
  return { held, state };
}

function renderOffer() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <OfferScreen id="o-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Answers one PATCH and lets the response reach the cache and the cell, which
// takes several promise turns after the request settles.
async function answer(held: Held, status?: number) {
  await act(async () => {
    held.answer(status);
  });
  for (let turn = 0; turn < 10; turn += 1) {
    await act(async () => {});
  }
}

type Kind = {
  testId: string;
  line?: Record<string, unknown>;
  clear?: boolean;
  // The box shows a reformatted amount after a blur, so the second typing
  // replaces it instead of extending it.
  clearSecond?: boolean;
  first: string;
  second: string;
  shown: string;
  // How the box reads once left, where it differs from what was typed.
  left?: string;
  patches: [Record<string, unknown>, Record<string, unknown>];
};

const KINDS: Kind[] = [
  {
    testId: "line-description-li-1",
    first: " plus",
    second: " travel",
    shown: "Consulting plus travel",
    patches: [
      { description: "Consulting plus" },
      { description: "Consulting plus travel" },
    ],
  },
  {
    testId: "line-quantity-li-1",
    first: "0",
    second: "5",
    shown: "1005",
    patches: [{ quantity: 100 }, { quantity: 1005 }],
  },
  {
    testId: "line-unit-price-li-1",
    clear: true,
    clearSecond: true,
    first: "12",
    second: "12.5",
    shown: "12.5",
    left: "12.50",
    patches: [{ unit_price_minor: 1200 }, { unit_price_minor: 1250 }],
  },
  {
    testId: "line-unit-li-1-other",
    line: { unit: "crate" },
    first: "s",
    second: "!",
    shown: "crates!",
    patches: [{ unit: "crates" }, { unit: "crates!" }],
  },
  {
    testId: "line-discount-li-1",
    clear: true,
    first: "7",
    second: "5",
    shown: "75",
    patches: [{ discount_pct: 7 }, { discount_pct: 75 }],
  },
  {
    testId: "line-tax-rate-li-1",
    clear: true,
    first: "7",
    second: "5",
    shown: "75",
    patches: [{ tax_rate: 7 }, { tax_rate: 75 }],
  },
];

async function typeInto(
  user: ReturnType<typeof userEvent.setup>,
  cell: HTMLInputElement,
  text: string,
  clear = false,
) {
  await user.click(cell);
  if (clear) {
    await user.clear(cell);
  }
  await user.type(cell, text);
}

async function setUp(kind: Kind) {
  const server = stubServer({ ...baseLine, ...kind.line });
  const user = userEvent.setup();
  renderOffer();
  await screen.findByText("ANG-2026-0007");
  const cell = screen.getByTestId(kind.testId) as HTMLInputElement;
  return { ...server, user, cell };
}

describe.each(KINDS)(
  "the $testId cell while its earlier save is on its way",
  (kind) => {
    it("keeps the text typed after the save started, and saves it after", async () => {
      const { held, state, user, cell } = await setUp(kind);
      await typeInto(user, cell, kind.first, kind.clear);
      await user.tab();
      await waitFor(() => expect(held).toHaveLength(1));
      expect(held[0].patch).toEqual(kind.patches[0]);

      await typeInto(user, cell, kind.second, kind.clearSecond);
      await answer(held[0]);
      expect(screen.getByTestId(kind.testId)).toBe(cell);
      expect(cell.value).toBe(kind.shown);
      expect(document.activeElement).toBe(cell);

      await user.tab();
      await waitFor(() => expect(held).toHaveLength(2));
      expect(held[1].patch).toEqual(kind.patches[1]);
      await answer(held[1]);
      await waitFor(() => expect(state.line).toMatchObject(kind.patches[1]));
      await waitFor(() => expect(cell.value).toBe(kind.left ?? kind.shown));
      expect(state.mostInflight).toBe(1);
    });

    it("sends newer text left before the first answer only after that answer", async () => {
      const { held, state, user, cell } = await setUp(kind);
      await typeInto(user, cell, kind.first, kind.clear);
      await user.tab();
      await waitFor(() => expect(held).toHaveLength(1));
      await typeInto(user, cell, kind.second, kind.clearSecond);
      await user.tab();

      await act(async () => {});
      expect(held).toHaveLength(1);
      await answer(held[0]);
      await waitFor(() => expect(held).toHaveLength(2));
      expect(held[1].patch).toEqual(kind.patches[1]);
      await answer(held[1]);
      await waitFor(() => expect(state.line).toMatchObject(kind.patches[1]));
      await waitFor(() => expect(cell.value).toBe(kind.left ?? kind.shown));
      expect(state.mostInflight).toBe(1);
    });

    it("keeps a refused newer text and shows the server's reason in its row", async () => {
      const { held, state, user, cell } = await setUp(kind);
      await typeInto(user, cell, kind.first, kind.clear);
      await user.tab();
      await waitFor(() => expect(held).toHaveLength(1));
      await typeInto(user, cell, kind.second, kind.clearSecond);
      await user.tab();
      await answer(held[0]);
      await waitFor(() => expect(held).toHaveLength(2));
      await answer(held[1], 422);

      const row = screen.getByTestId("line-row-li-1");
      expect(await within(row).findByText(REFUSAL)).toBeTruthy();
      expect(screen.queryByText("Change was not saved")).toBeNull();
      expect(cell.value).toBe(kind.left ?? kind.shown);
      expect(screen.getByTestId(kind.testId)).toBe(cell);
      expect(state.line).toMatchObject(kind.patches[0]);
    });
  },
);

describe("an offer line cell", () => {
  it("takes the saved value when nothing newer was typed", async () => {
    const { held, user, cell } = await setUp(KINDS[0]);
    await typeInto(user, cell, "!");
    await user.tab();
    await waitFor(() => expect(held).toHaveLength(1));
    await answer(held[0]);
    await waitFor(() => expect(cell.value).toBe("Consulting!"));
    expect(held).toHaveLength(1);
    expect(screen.queryByText("Change was not saved")).toBeNull();
  });
});

describe("the billing cell", () => {
  it("keeps a refused term, shows the server's reason in its row, and sends it again on the next blur", async () => {
    const { held, state } = stubServer({
      ...baseLine,
      billing_model: "recurring",
      billing_interval_months: 3,
      interval_count: 4,
    });
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");
    const term = screen.getByRole("spinbutton", { name: "Periods, line 1" });
    const refusedTerm = {
      billing_model: "recurring",
      billing_interval_months: 3,
      interval_count: 0,
    };

    await typeInto(user, term as HTMLInputElement, "0", true);
    await user.tab();
    await waitFor(() => expect(held).toHaveLength(1));
    expect(held[0].patch).toEqual(refusedTerm);
    await answer(held[0], 422);

    const row = screen.getByTestId("line-row-li-1");
    expect(await within(row).findByText(REFUSAL)).toBeTruthy();
    expect(screen.queryByText("Change was not saved")).toBeNull();
    expect(screen.getByRole("spinbutton", { name: "Periods, line 1" })).toBe(
      term,
    );
    expect(term).toHaveProperty("value", "0");
    expect(term.getAttribute("aria-invalid")).toBe("true");

    await user.click(term);
    await user.tab();
    await waitFor(() => expect(held).toHaveLength(2));
    expect(held[1].patch).toEqual(refusedTerm);
    await answer(held[1]);
    await waitFor(() => expect(within(row).queryByText(REFUSAL)).toBeNull());
    expect(term.getAttribute("aria-invalid")).toBeNull();
    expect(state.line).toMatchObject({ interval_count: 0 });
  });
});
