// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { expectReadable } from "../readable.testkit";
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
const figures = (root: HTMLElement) =>
  [...root.querySelectorAll(".stat-card")].map((c) => c.textContent);

beforeEach(() => {
  asked.length = 0;
});

describe("the attention card for slipping deals", () => {
  it("ranks the deals with their reasons and what is at stake", () => {
    const root = mount(slippingFixture.data);
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe("Slipping this week");
    expect(names(root)).toEqual(["Acme renewal", "Nordwind rollout"]);
    expect(figures(root)[0]).toBe("Deals slipping2");
    expect(figures(root)[1]).toMatch(/^Value at stake.*97,?000/);
    expect(
      [...root.querySelectorAll(".attention-reason")].map((r) => r.textContent),
    ).toEqual([
      "no recorded activity since 2026-06-16",
      "no open next step",
      "expected to close on 2026-09-30",
    ]);
  });

  it("totals no value across currencies", () => {
    const [first, second] = (slippingFixture.data as { deals: object[] }).deals;
    const root = mount({ deals: [first, { ...second, currency: "USD" }] });
    expect(figures(root)[1]).toBe("Value at stakeMixedIn 2 currencies");
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
    expect(figures(root)).toEqual(["Deals checked42", "At risk1"]);
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
  });
});

describe("the attention card for open promises", () => {
  it("counts the promises by state and marks the late and the unowned", () => {
    const root = mount(commitmentsFixture.data);
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe("Open promises");
    expect(figures(root)).toEqual([
      "Overdue1",
      "Coming up0",
      "No date1",
      "Nobody owns1",
    ]);
    expect(root.textContent).toContain("6 days late");
    expect(root.textContent).toContain("Nobody owns this");
    expect(root.textContent).toContain("For Acme GmbH");
  });

  it("calls a promise hours late late, not due", () => {
    const [late] = (commitmentsFixture.data as { commitments: object[] })
      .commitments;
    const root = mount({
      as_of: "2026-10-30T00:00:00Z",
      commitments: [{ ...late, days_overdue: 0 }],
    });
    expect(root.querySelector(".badge-danger")?.textContent).toBe("Late");
  });
});

describe("an answer the card does not know", () => {
  it("says there is nothing to show", () => {
    const root = mount({ something: "else" });
    expect(root.querySelector("h1")?.textContent).toBe("Nothing to show");
  });
});
