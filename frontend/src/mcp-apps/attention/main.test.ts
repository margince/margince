// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { expectReadable } from "../readable.testkit";
import { compactMoney } from "../values";
import { atRiskFixture, commitmentsFixture, slippingFixture } from "./fixture";

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

const names = (root: HTMLElement) =>
  [...root.querySelectorAll(".attention-row .name")].map((n) => n.textContent);
const lead = (root: HTMLElement) =>
  root.querySelector(".view-hero-figure")?.textContent;
const legend = (root: HTMLElement) =>
  [...root.querySelectorAll(".view-legend-item")].map((i) => i.textContent);

beforeEach(() => {
  asked.length = 0;
});

describe("the attention card for slipping deals", () => {
  it("ranks the deals with their reasons and what is at stake", () => {
    const root = mount(slippingFixture.data);
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe("Slipping this week");
    expect(names(root)).toEqual(["Acme renewal", "Nordwind rollout"]);
    expect(lead(root)).toBe(`${compactMoney(9_700_000, "EUR")}at stake`);
    expect(root.querySelector(".view-hero-sentence")?.textContent).toBe(
      "2 deals are slipping. 1 has gone quiet.",
    );
    expect(legend(root)).toEqual([
      `Acme renewal${compactMoney(7_200_000, "EUR")}`,
      `Nordwind rollout${compactMoney(2_500_000, "EUR")}`,
    ]);
    expect(
      [...root.querySelectorAll(".attention-reason")].map((r) => r.textContent),
    ).toEqual([
      "no recorded activity since 2026-06-16",
      "no open next step",
      "expected to close on 2026-09-30",
    ]);
  });

  it("states no total while a deal has no amount", () => {
    const [first, second] = (slippingFixture.data as { deals: object[] }).deals;
    const root = mount({ deals: [first, { ...second, amount_minor: null }] });
    expect(lead(root)).toBe("2deals slipping");
  });

  it("totals no value across currencies", () => {
    const [first, second] = (slippingFixture.data as { deals: object[] }).deals;
    const root = mount({ deals: [first, { ...second, currency: "USD" }] });
    expect(lead(root)).toBe("2deals slipping");
    expect(root.querySelector(".view-segments")).toBeNull();
  });

  it("puts a catch-up question about the deal in the chat", () => {
    const root = mount(slippingFixture.data);
    root.querySelector<HTMLButtonElement>(".attention-row button")?.click();
    expect(asked).toHaveLength(1);
    expect(asked[0]).toContain('"Acme renewal"');
    expect(asked[0]).toContain("0195c3a0-0000-7000-8000-0000000000d1");
  });

  it("says nothing slips in an empty week", () => {
    const root = mount({ deals: [] });
    expectReadable(root);
    expect(root.querySelector(".intro")?.textContent).toBe(
      "No deal is slipping this week.",
    );
  });
});

describe("the attention card for relationships at risk", () => {
  it("names each rule a deal breaks and who it rests on", () => {
    const root = mount(atRiskFixture.data);
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe("Relationships at risk");
    expect(root.textContent).toContain(
      "Only one engaged contact on their side (Lena Vogt)",
    );
    expect(lead(root)).toBe("1of 42 deals at risk");
    expect(legend(root)).toEqual(["At risk1", "Covered41"]);
  });

  it("says the check stopped short rather than calling the rest clean", () => {
    const root = mount({
      ...(atRiskFixture.data as object),
      deals: [],
      truncated: true,
    });
    expect(root.querySelector(".readings-floor")?.textContent).toContain(
      "stopped after 42 deals",
    );
    expect(root.querySelector(".view-hero-sentence")?.textContent).toBe(
      "None of the deals checked rests on one contact or has gone quiet.",
    );
  });
});

describe("the attention card for open promises", () => {
  it("counts the promises by state and marks the late and the unowned", () => {
    const root = mount(commitmentsFixture.data);
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe("Open promises");
    expect(lead(root)).toBe("1overdue");
    expect(root.querySelector(".view-ring")?.textContent).toBe("2open");
    expect(legend(root)).toEqual(["Overdue1", "Coming up0", "No date1"]);
    expect(root.querySelector(".view-hero-sentence")?.textContent).toBe(
      "1 promise has nobody to keep it.",
    );
    expect(root.textContent).toContain("6 days late");
    expect(root.textContent).toContain("Nobody owns this");
    expect(root.textContent).toContain("For Acme GmbH");
  });

  it("names the record a promise lives on in its catch-up question", () => {
    const root = mount(commitmentsFixture.data);
    for (const button of root.querySelectorAll<HTMLButtonElement>(
      ".attention-row button",
    )) {
      button.click();
    }
    expect(asked[0]).toContain("claim_id 0195c3a0-0000-7000-8000-0000000000f1");
    expect(asked[1]).toContain("task_id 0195c3a0-0000-7000-8000-0000000000f2");
  });

  it("calls a promise hours late late, not due", () => {
    const [late] = (commitmentsFixture.data as { commitments: object[] })
      .commitments;
    const root = mount({
      as_of: "2026-10-30T00:00:00Z",
      commitments: [{ ...late, days_overdue: 0 }],
    });
    expect(
      root.querySelector(".attention-status .badge-danger")?.textContent,
    ).toBe("Late");
  });
});

describe("an answer the card does not know", () => {
  it("says there is nothing to show", () => {
    const root = mount({ something: "else" });
    expect(root.querySelector("h1")?.textContent).toBe("Nothing to show");
  });
});
