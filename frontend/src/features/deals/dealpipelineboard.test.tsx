/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  type BoundFunctions,
  cleanup,
  type queries,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  BoardDeal,
  BoardMoneyColumn,
} from "../../design-system/pipelineboard";
import { LocaleProvider } from "../../i18n";
import {
  type WriteTo,
  WriteToProvider,
  type WriteToTarget,
} from "../../lib/writeto";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { DealPipelineBoard } from "./dealpipelineboard";
import { deal, stages as openThenWon, type Stage } from "./deals.testkit";

// What a card on the deals board offers without leaving the board, and that
// each verb opens what the deal page opens rather than a copy of it.

const openDeal: BoardDeal = {
  id: "d1",
  name: "Fleet retrofit",
  company: "",
  companyId: "",
  valueMinor: 4_800_000,
  currency: "EUR",
  idleSince: "2026-09-15T12:00:00Z",
  measuredAt: Date.parse("2026-09-15T12:00:00Z"),
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

// The grants the verbs wait on, and nothing else: every other read the
// summary drawer makes answers 404, which its own panels already state.
function stubGrants(allow: GrantSpec) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      return jsonResponse({ title: "Not found", status: 404 }, 404);
    }),
  );
}

beforeEach(() => {
  stubGrants({ activity: ["create"] });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderBoard(
  deals: BoardDeal[],
  writeTo: WriteTo | null,
  menu?: ComponentProps<typeof DealPipelineBoard>["menu"],
) {
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
      menu={menu}
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
    await user.click(
      screen.getByRole("button", { name: "Deal summary: Fleet retrofit" }),
    );
    const drawer = await screen.findByRole("dialog", {
      name: "Fleet retrofit",
    });
    expect(
      within(drawer)
        .getByRole("link", { name: "Open deal" })
        .getAttribute("href"),
    ).toBe("/#/deals/d1");
  });

  it("writes to the deal through the shell's one composer", async () => {
    const user = userEvent.setup();
    const asked: WriteToTarget[] = [];
    renderBoard([openDeal], (target) => {
      asked.push(target);
    });
    await user.click(
      screen.getByRole("button", { name: "Write email: Fleet retrofit" }),
    );
    expect(asked).toEqual([{ entityType: "deal", entityId: "d1" }]);
  });

  // No mailbox the product can send from means no composer to open, and a
  // verb that opened onto a refusal would be a press that does nothing.
  it("offers no mail verb to a reader with no mailbox", () => {
    renderBoard([openDeal], null);
    expect(screen.queryByRole("button", { name: /^Write email/ })).toBeNull();
  });

  it("files a task in the deal page's own task drawer once the grant is known", async () => {
    const user = userEvent.setup();
    renderBoard([openDeal], null);
    await user.click(
      await screen.findByRole("button", { name: "Add task: Fleet retrofit" }),
    );
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
      name: "Add task: Fleet retrofit",
    });
    const archived = within(card(container, "d2"));
    expect(
      archived.getByRole("button", { name: "Deal summary: Old retrofit" }),
    ).toBeTruthy();
    expect(archived.queryByRole("button", { name: /^Write email/ })).toBeNull();
    expect(archived.queryByRole("button", { name: /^Add task/ })).toBeNull();
  });

  describe("card menu", () => {
    const lost: Stage = {
      id: "s4",
      pipeline_id: "pl",
      name: "Lost",
      position: 4,
      semantic: "lost",
      win_probability: 0,
    };
    const pipeline = [...openThenWon, lost];
    const onAdvance = vi.fn(() => Promise.resolve(false));
    afterEach(() => onAdvance.mockClear());
    beforeEach(() => {
      stubGrants({ activity: ["create"], deal: ["read", "update", "delete"] });
    });

    // The panel, once the grants have answered: the task verb waits on them.
    async function openMenu(name: string) {
      const user = userEvent.setup();
      const trigger = screen.getByRole("button", {
        name: `More actions for ${name}`,
      });
      await user.click(trigger);
      const panel = document.getElementById(
        trigger.getAttribute("aria-controls") ?? "",
      );
      if (!panel) throw new Error(`no menu for ${name}`);
      await within(panel).findByRole("button", { name: "Add task" });
      return { user, panel: within(panel) };
    }
    const itemNames = (panel: BoundFunctions<typeof queries>) =>
      panel.getAllByRole("button").map((item) => item.textContent);

    it("offers an open deal its moves, then the card's verbs and the record's", async () => {
      renderBoard([openDeal], null, {
        deals: [deal({ archivable: true })],
        stages: pipeline,
        onAdvance,
      });
      const { panel } = await openMenu("Fleet retrofit");
      expect(itemNames(panel)).toEqual([
        "Move to Proposal",
        "Move to stage",
        "Mark won",
        "Mark lost",
        "Change close date",
        "Change owner",
        "Deal summary",
        "Add task",
        "Copy link",
        "Archive deal",
      ]);
    });

    it("hands the next stage to the board's one advance", async () => {
      renderBoard([openDeal], null, {
        deals: [deal({})],
        stages: pipeline,
        onAdvance,
      });
      const { user, panel } = await openMenu("Fleet retrofit");
      await user.click(panel.getByRole("button", { name: "Move to Proposal" }));
      expect(onAdvance).toHaveBeenCalledWith("d1", "s2");
    });

    it("hands a stage picked from the list to the board's one advance", async () => {
      renderBoard([openDeal], null, {
        deals: [deal({})],
        stages: pipeline,
        onAdvance,
      });
      const { user, panel } = await openMenu("Fleet retrofit");
      await user.click(panel.getByRole("button", { name: "Move to stage" }));
      await user.click(await screen.findByRole("option", { name: "Won" }));
      expect(onAdvance).toHaveBeenCalledWith("d1", "s3");
    });

    // The deal page refuses to move a closed deal and offers Reopen; the card
    // points there.
    it("refuses a closed deal's moves and says where it is reopened", async () => {
      renderBoard([openDeal], null, {
        deals: [deal({ status: "won", stage_id: "s3" })],
        stages: pipeline,
        onAdvance,
      });
      const { panel } = await openMenu("Fleet retrofit");
      const reason = "This deal is closed. Reopen it on its page to move it.";
      expect(panel.getByText(reason)).toBeTruthy();
      for (const name of ["Move to stage", "Mark lost"]) {
        const item = panel.getByRole("button", { name });
        expect(item).toHaveProperty("disabled", true);
        expect(item.getAttribute("aria-describedby")).toBe(
          panel.getByText(reason).id,
        );
      }
      expect(panel.queryByRole("button", { name: "Mark won" })).toBeNull();
    });

    it("offers an archived deal its summary and its link alone", async () => {
      const user = userEvent.setup();
      renderBoard([archivedDeal], null, {
        deals: [deal({ id: "d2", archived_at: "2026-09-01T00:00:00Z" })],
        stages: pipeline,
        onAdvance,
      });
      const trigger = screen.getByRole("button", {
        name: "More actions for Old retrofit",
      });
      await user.click(trigger);
      const panel = within(
        document.getElementById(trigger.getAttribute("aria-controls") ?? "") ??
          document.body,
      );
      expect(itemNames(panel)).toEqual(["Deal summary", "Copy link"]);
    });

    // The deal page's rule: a seat that may not change the deal is offered
    // none of its writes, even where the row itself is archivable.
    it("offers no move and no archive to a seat that may not change the deal", async () => {
      renderBoard([openDeal], null, {
        deals: [deal({ writable: false, archivable: true })],
        stages: pipeline,
        onAdvance,
      });
      const { panel } = await openMenu("Fleet retrofit");
      expect(itemNames(panel)).toEqual([
        "Deal summary",
        "Add task",
        "Copy link",
      ]);
    });

    // One item per destination: the close says more than the bare move.
    it("offers the next stage as its close when it is the won stage", async () => {
      renderBoard([openDeal], null, {
        deals: [deal({ stage_id: "s2" })],
        stages: pipeline,
        onAdvance,
      });
      const { panel } = await openMenu("Fleet retrofit");
      expect(panel.queryByRole("button", { name: "Move to Won" })).toBeNull();
      expect(itemNames(panel).slice(0, 3)).toEqual([
        "Move to stage",
        "Mark won",
        "Mark lost",
      ]);
    });

    // With two won stages, no one stage is what "won" means.
    it("offers Mark won only where one stage means won", async () => {
      const secondWin: Stage = {
        id: "s5",
        pipeline_id: "pl",
        name: "Won with partner",
        position: 5,
        semantic: "won",
        win_probability: 100,
      };
      renderBoard([openDeal], null, {
        deals: [deal({})],
        stages: [...pipeline, secondWin],
        onAdvance,
      });
      const { panel } = await openMenu("Fleet retrofit");
      expect(panel.queryByRole("button", { name: "Mark won" })).toBeNull();
      expect(panel.getByRole("button", { name: "Mark lost" })).toBeTruthy();
    });
  });
});
