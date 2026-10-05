/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import type { BoardDeal, BoardMoneyColumn } from "../design-system/composed";
import { LocaleProvider } from "../i18n";
import { DealPipelineBoard } from "./dealpipelineboard";
import { type WriteTo, WriteToProvider, type WriteToTarget } from "./writeto";

// What a card on the deals board offers without leaving the board, and that
// each verb opens what the deal page opens rather than a copy of it.

const openDeal: BoardDeal = {
  id: "d1",
  name: "Fleet retrofit",
  company: "",
  companyId: "",
  valueMinor: 4_800_000,
  currency: "EUR",
  ageMs: 0,
};
const archivedDeal: BoardDeal = {
  ...openDeal,
  id: "d2",
  name: "Old retrofit",
  archived: true,
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  // The grant the task verb waits on, and nothing else: every other read the
  // summary drawer makes answers 404, which its own panels already state.
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow: { activity: ["create"] } }));
      }
      return jsonResponse({ title: "Not found", status: 404 }, 404);
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderBoard(deals: BoardDeal[], writeTo: WriteTo | null) {
  const column: BoardMoneyColumn = {
    stage: "s1",
    label: "Proposal",
    probabilityPct: 40,
    rawMinor: 4_800_000,
    weightedMinor: 1_920_000,
    currency: "EUR",
    deals,
  };
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui: ReactNode = (
    <DealPipelineBoard
      columns={[column]}
      cardHref={(deal) => `#/deals/${deal.id}`}
      zone="Europe/Berlin"
    />
  );
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <WriteToProvider writeTo={writeTo}>{ui}</WriteToProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function card(container: HTMLElement, dealId: string): HTMLElement {
  const found = container.querySelector<HTMLElement>(`[data-deal="${dealId}"]`);
  if (!found) {
    throw new Error(`no card for deal ${dealId}`);
  }
  return found;
}

describe("DealPipelineBoard", () => {
  it("opens a deal's summary beside the board, with the way into the deal", async () => {
    const user = userEvent.setup();
    renderBoard([openDeal], null);
    await user.click(screen.getByRole("button", { name: "Deal summary" }));
    const drawer = await screen.findByRole("dialog", {
      name: "Fleet retrofit",
    });
    expect(
      within(drawer)
        .getByRole("link", { name: "Open deal" })
        .getAttribute("href"),
    ).toBe("#/deals/d1");
  });

  it("writes to the deal through the shell's one composer", async () => {
    const user = userEvent.setup();
    const asked: WriteToTarget[] = [];
    renderBoard([openDeal], (target) => {
      asked.push(target);
    });
    await user.click(screen.getByRole("button", { name: "Email" }));
    expect(asked).toEqual([{ entityType: "deal", entityId: "d1" }]);
  });

  // No mailbox the product can send from means no composer to open, and a
  // verb that opened onto a refusal would be a press that does nothing.
  it("offers no mail verb to a reader with no mailbox", () => {
    renderBoard([openDeal], null);
    expect(screen.queryByRole("button", { name: "Email" })).toBeNull();
  });

  it("files a task in the deal page's own task drawer once the grant is known", async () => {
    const user = userEvent.setup();
    renderBoard([openDeal], null);
    await user.click(await screen.findByRole("button", { name: "Add task" }));
    expect(
      await screen.findByRole("dialog", { name: "Add task" }),
    ).toBeTruthy();
  });

  // An archived deal takes no new mail and no new work. The open card beside
  // it proves the grant has answered, so the absence is the archive's refusal
  // and not a read still in flight.
  it("offers an archived deal its summary and nothing that writes to it", async () => {
    const { container } = renderBoard([openDeal, archivedDeal], () => {});
    await within(card(container, "d1")).findByRole("button", {
      name: "Add task",
    });
    const archived = within(card(container, "d2"));
    expect(archived.getByRole("button", { name: "Deal summary" })).toBeTruthy();
    expect(archived.queryByRole("button", { name: "Email" })).toBeNull();
    expect(archived.queryByRole("button", { name: "Add task" })).toBeNull();
  });
});
