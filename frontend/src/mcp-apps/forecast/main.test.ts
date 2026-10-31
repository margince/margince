// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { expectReadable } from "../readable.testkit";
import { compactMoney } from "../values";
import { checksFixture, movementFixture, readingsFixture } from "./fixture";

const asked: string[] = [];

vi.mock("../actions", () => ({
  askAssistant: (text: string) => {
    asked.push(text);
  },
}));

const { render } = await import("./main");

function mount(data: unknown): HTMLElement {
  const root = document.createElement("main");
  document.body.replaceChildren(root);
  render(root, data, []);
  return root;
}

const lead = (root: HTMLElement) =>
  root.querySelector(".view-hero-figure")?.textContent;
const bars = (root: HTMLElement) =>
  [...root.querySelectorAll(".view-bar")].map((b) => ({
    label: b.querySelector(".view-bar-label")?.textContent,
    figure: b.querySelector(".view-bar-figure")?.textContent,
    width: b.querySelector<HTMLElement>(".view-share span")?.style.width,
  }));

beforeEach(() => {
  asked.length = 0;
});

describe("the forecast card for a period's readings", () => {
  it("leads with the weighted figure and draws each reading against the open pipeline", () => {
    const root = mount(readingsFixture.data);
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe("The forecast");
    expect(root.querySelector(".panel-head .badge")?.textContent).toBe(
      "2026-10-01 to 2026-12-31",
    );
    expect(lead(root)).toBe(`${compactMoney(7_300_000, "EUR")}weighted`);
    expect(bars(root)).toEqual([
      { label: "Won", figure: compactMoney(4_000_000, "EUR"), width: "33%" },
      {
        label: "With evidence",
        figure: compactMoney(6_500_000, "EUR"),
        width: "54%",
      },
      {
        label: "Best case",
        figure: compactMoney(9_000_000, "EUR"),
        width: "75%",
      },
      { label: "Open", figure: compactMoney(12_000_000, "EUR"), width: "100%" },
    ]);
    expect(root.querySelector(".readings-floor")?.textContent).toBe(
      "2 of 14 deals have no amount and are in no total.",
    );
  });

  it("states the gap itself when the server sent no note", () => {
    const { coverage_note: _, ...bare } = readingsFixture.data as Record<
      string,
      unknown
    >;
    const root = mount(bare);
    expect(root.querySelector(".readings-floor")?.textContent).toBe(
      "2 of 14 deals have no amount yet.",
    );
  });

  it("asks the assistant what moved the period", () => {
    const root = mount(readingsFixture.data);
    root.querySelector<HTMLButtonElement>(".row-action")?.click();
    expect(asked).toEqual([
      expect.stringContaining("2026-10-01 to 2026-12-31"),
    ]);
  });
});

describe("the forecast card for a movement", () => {
  it("states the change and its causes as shares of the opening, biggest first", () => {
    const root = mount(movementFixture.data);
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe(
      "What moved the forecast",
    );
    expect(lead(root)).toBe("+12%since the opening snapshot");
    expect(root.querySelector(".view-hero-sentence")?.textContent).toBe(
      "New deals moved it most.",
    );
    expect(bars(root).map((b) => [b.label, b.figure])).toEqual([
      ["New deals (2 deals)", "+20%"],
      ["Pushed out of the period (1 deal)", "-10%"],
      ["Value changes (1 deal)", "+2%"],
    ]);
  });

  it("names no money, because the answer carries no currency", () => {
    const root = mount(movementFixture.data);
    expect(root.textContent).not.toMatch(/€|EUR/);
  });

  it("counts deals when the opening figure was nothing", () => {
    const root = mount({
      ...(movementFixture.data as object),
      opening_minor: 0,
    });
    expect(lead(root)).toBe("4deals moved");
    expect(bars(root)[0]?.figure).toBe("2");
  });
});

describe("the forecast card for the input checks", () => {
  it("gives the verdict and how much was read", () => {
    const root = mount(checksFixture.data);
    expectReadable(root);
    expect(root.querySelector(".panel-head .badge")?.textContent).toBe(
      "Ready, with exceptions",
    );
    expect(root.querySelector(".intro")?.textContent).toBe(
      "The checks passed, with a few deals to look at. The check read 14 deals.",
    );
  });
});

describe("the forecast card when records moved after the check", () => {
  it("says the verdict may no longer hold instead of stating it as current", () => {
    const root = mount({
      ...(checksFixture.data as object),
      readiness: "ready",
      freshness: { state: "out_of_date", changed_deals: 2 },
    });
    expect(root.querySelector(".panel-head .badge")?.textContent).toBe(
      "Out of date",
    );
    expect(root.querySelector(".intro")?.textContent).toBe(
      "Records changed after the last check read them (2 deals), so its verdict, ready, may no longer hold. The check read 14 deals.",
    );
  });
});

describe("the forecast card for a caller who cannot open every deal", () => {
  it("says the totals leave those deals out", () => {
    const root = mount({
      ...(readingsFixture.data as object),
      scope_limited: true,
    });
    expect(root.textContent).toContain(
      "Deals you cannot open are left out of these totals.",
    );
  });

  it("says nothing of it when every deal counts", () => {
    expect(mount(readingsFixture.data).textContent).not.toContain(
      "left out of these totals",
    );
  });
});

describe("the forecast card for the open findings", () => {
  it("lists each finding with what is at stake", () => {
    const root = mount({
      data: [
        {
          type: "stale_close_date",
          severity: "high",
          affected_minor: 1_200_000,
          currency: "EUR",
        },
        { type: "missing_amount", severity: "low" },
      ],
    });
    expectReadable(root);
    expect(root.querySelector(".panel-head .badge")?.textContent).toBe(
      "2 findings",
    );
    const lines = [...root.querySelectorAll(".context-line")].map((line) =>
      line.textContent,
    );
    expect(lines).toEqual([
      `Stale close dateHigh, ${compactMoney(1_200_000, "EUR")} at stake`,
      "Missing amountLow",
    ]);
  });
});

describe("the forecast card for the sources a check read", () => {
  it("names each source and how far it was read", () => {
    const root = mount({
      as_of: "2026-10-30T03:00:00Z",
      sources: [
        { source: "deals", state: "checked", checked_through: "2026-10-29" },
        { source: "signals", state: "unreadable" },
      ],
    });
    expectReadable(root);
    const metas = [...root.querySelectorAll(".context-line .meta")].map(
      (m) => m.textContent,
    );
    expect(metas[0]).toMatch(/^Checked through /);
    expect(metas[1]).toBe("Unreadable");
  });
});

describe("an answer the card does not know", () => {
  it("says there is nothing to show", () => {
    expect(mount({ something: "else" }).querySelector("h1")?.textContent).toBe(
      "Nothing to show",
    );
  });
});
