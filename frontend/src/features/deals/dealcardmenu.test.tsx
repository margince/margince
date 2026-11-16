/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { askedToClose } from "./deal360/confirmadvance.testkit";
import { DealsScreen } from "./deals";
import { deal, render, stubBackend } from "./deals.testkit";

// A card's menu moves a deal through the board's one advance, as a drop does:
// the same pinned version, and the same confirm before a terminal stage.

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

    const proposal = await waitFor(() => {
      const column = document.querySelector<HTMLElement>('[data-stage="s2"]');
      if (!column) throw new Error("no Proposal column");
      return within(column).getByRole("link", { name: "Fleet retrofit" });
    });
    await waitFor(() => expect(document.activeElement).toBe(proposal));
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
    await user.click(await menu.findByRole("button", { name: "Mark won…" }));

    await askedToClose("Won");
    expect(advances).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual([{ to_stage_id: "s3", status: "won" }, "4"]);
  });
});
