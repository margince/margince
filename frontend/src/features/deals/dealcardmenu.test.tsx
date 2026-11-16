/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { askedToClose } from "./deal360/confirmadvance.testkit";
import { DealsScreen } from "./deals";
import { deal, jsonResponse, render, stubBackend } from "./deals.testkit";

// A card's menu moves a deal as a drop does: pinned, and confirmed when terminal.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

async function openCardMenu(user: ReturnType<typeof userEvent.setup>) {
  const trigger = await screen.findByRole("button", {
    name: "More actions for Fleet retrofit",
  });
  await user.click(trigger);
  const panel = document.getElementById(
    trigger.getAttribute("aria-controls") ?? "",
  );
  if (!panel) throw new Error("the card's menu drew no panel");
  return within(panel);
}

// The deal's link, once the board draws it in that stage's column.
function cardLinkIn(stageId: string) {
  return waitFor(() => {
    const column = document.querySelector<HTMLElement>(
      `[data-stage="${stageId}"]`,
    );
    if (!column) throw new Error(`no column for stage ${stageId}`);
    return within(column).getByRole("link", { name: "Fleet retrofit" });
  });
}

// The board's drag, which happy-dom cannot perform, as its drop handler sees it.
function dropOnStage(stageId: string) {
  const drop = new Event("drop", { bubbles: true });
  Object.assign(drop, {
    dataTransfer: { getData: () => "d1", setData: () => undefined },
  });
  document.querySelector(`[data-stage="${stageId}"]`)?.dispatchEvent(drop);
}

describe("deal card menu on the deals board", () => {
  it("moves a deal to the next stage with the version it was drawn from", async () => {
    const advances: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onAdvance: (b, m) => advances.push([b, m]) }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(
      await menu.findByRole("button", { name: "Move to Proposal" }),
    );

    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual([{ to_stage_id: "s2" }, "4"]);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(await screen.findByText("Moved to Proposal")).toBeTruthy();
  });

  // The card is drawn anew in its new column; a keyboard reader stays on it.
  it("keeps focus on the deal it moved", async () => {
    const rows = [deal({})];
    vi.stubGlobal(
      "fetch",
      stubBackend(rows, {
        onAdvance: () => {
          rows[0] = deal({ stage_id: "s2", version: 5 });
        },
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    (await menu.findByRole("button", { name: "Move to Proposal" })).focus();
    await user.keyboard("{Enter}");

    const proposal = await cardLinkIn("s2");
    await waitFor(() => expect(document.activeElement).toBe(proposal));
  });

  // A move put away or refused is no move: a later drag that lands the deal
  // there leaves focus where it was.
  it("does not follow a deal whose terminal move was cancelled", async () => {
    const rows = [deal({})];
    vi.stubGlobal(
      "fetch",
      stubBackend(rows, {
        onAdvance: () => {
          rows[0] = deal({ stage_id: "s3", status: "won", version: 5 });
        },
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(await menu.findByRole("button", { name: "Mark won" }));
    await askedToClose("Won");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    dropOnStage("s3");
    await askedToClose("Won");
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    expect(document.activeElement).not.toBe(await cardLinkIn("s3"));
  });

  it("does not follow a deal whose move was refused", async () => {
    const rows = [deal({})];
    const backend = stubBackend(rows, {
      onAdvance: () => {
        rows[0] = deal({ stage_id: "s2", version: 5 });
      },
    });
    let refused = false;
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : String(input);
      if (!refused && url.includes("/advance")) {
        refused = true;
        return Promise.resolve(
          jsonResponse({ status: 409, title: "Conflict" }, 409),
        );
      }
      return backend(input, init);
    });
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(
      await menu.findByRole("button", { name: "Move to Proposal" }),
    );
    await waitFor(() => expect(refused).toBe(true));

    dropOnStage("s2");
    expect(document.activeElement).not.toBe(await cardLinkIn("s2"));
  });

  it("asks before marking a deal won, and posts only after the confirm", async () => {
    const advances: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onAdvance: (b, m) => advances.push([b, m]) }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(await menu.findByRole("button", { name: "Mark won" }));

    await askedToClose("Won");
    expect(advances).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual([{ to_stage_id: "s3", status: "won" }, "4"]);
  });

  it("closes the menu behind the confirm when Won is picked from the stage list", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(menu.getByRole("button", { name: "Move to stage" }));
    await user.click(await screen.findByRole("option", { name: "Won" }));

    await askedToClose("Won");
    const trigger = screen.getByRole("button", {
      name: "More actions for Fleet retrofit",
    });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });
});
