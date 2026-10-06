// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { duplicateFixture } from "./fixture";

const calls: { tool: string; args: Record<string, unknown> }[] = [];
const asked: string[] = [];
let proxies = true;
let answer:
  | { ok: true; data: unknown; warnings: [] }
  | { ok: false; reason: string } = {
  ok: true,
  data: {},
  warnings: [],
};

vi.mock("../actions", () => ({
  declareActions: vi.fn(),
  canCallTools: () => proxies,
  callServerTool: async (tool: string, args: Record<string, unknown>) => {
    calls.push({ tool, args });
    return answer;
  },
  askAssistant: (text: string) => {
    asked.push(text);
  },
}));

const { render } = await import("./main");

function mount(data: unknown = duplicateFixture.data): HTMLElement {
  const root = document.createElement("main");
  document.body.replaceChildren(root);
  render(root, data, []);
  return root;
}

const labels = (root: HTMLElement) =>
  [...root.querySelectorAll("button")].map((b) => b.textContent);

async function press(root: HTMLElement, label: string) {
  const target = [...root.querySelectorAll("button")].find(
    (b) => b.textContent === label,
  );
  target?.click();
  await new Promise((r) => setTimeout(r, 0));
}

beforeEach(() => {
  calls.length = 0;
  asked.length = 0;
  proxies = true;
  answer = { ok: true, data: {}, warnings: [] };
});

describe("the duplicate card", () => {
  it("draws nothing when the create filed no candidate", () => {
    const root = mount({ record_type: "contact", id: "x", fields: {} });
    expect(root.childElementCount).toBe(0);
  });

  it("shows the two records' values side by side, the new one first", () => {
    const root = mount();
    expect(
      [...root.querySelectorAll("thead th")].map((n) => n.textContent),
    ).toEqual(["", "New record", "Already on file"]);
    const phone = [...root.querySelectorAll("tbody tr")][1];
    expect([...phone.children].map((n) => n.textContent)).toEqual([
      "phone",
      "+49 30 1234567",
      "+49301234567",
    ]);
    expect(root.querySelector(".intro")?.textContent).toBe(
      "Anna Meyer looks like a record already on file.",
    );
  });

  it("merges the new record into the one already on file", async () => {
    const root = mount();
    await press(root, "Merge into the existing record");
    expect(calls).toEqual([
      {
        tool: "merge_records",
        args: {
          record_type: "contact",
          source_id: "0195c3a0-0000-7000-8000-000000000002",
          target_id: "0195c3a0-0000-7000-8000-000000000001",
        },
      },
    ]);
  });

  it("settles on a merge and keeps the settled state when the host redelivers the result", async () => {
    const root = mount();
    await press(root, "Merge into the existing record");
    expect(root.textContent).toContain(
      "Merged into the record already on file.",
    );
    expect(labels(root)).toEqual([]);
    render(root, duplicateFixture.data, []);
    expect(root.textContent).toContain(
      "Merged into the record already on file.",
    );
  });

  it("dismisses the pair and offers an undo that re-opens it", async () => {
    const root = mount();
    await press(root, "Not the same");
    expect(calls[0]).toEqual({
      tool: "decide_duplicate",
      args: {
        candidate_id: "0195c3a0-0000-7000-8000-0000000000aa",
        decision: "not_the_same",
      },
    });
    expect(root.textContent).toContain("Marked as not the same.");
    await press(root, "Undo");
    expect(calls[1]).toEqual({
      tool: "decide_duplicate",
      args: {
        candidate_id: "0195c3a0-0000-7000-8000-0000000000aa",
        decision: "reopen",
      },
    });
    expect(labels(root)).toContain("Not the same");
  });

  it("says why when the tool refuses, and offers the choice again", async () => {
    answer = { ok: false, reason: "Both companies have projects." };
    const root = mount();
    await press(root, "Merge into the existing record");
    expect(root.querySelector(".refusal")?.textContent).toBe(
      "Both companies have projects.",
    );
    expect(labels(root)).toContain("Merge into the existing record");
  });

  it("offers no merge for a record type that has none", () => {
    const root = mount({
      ...(duplicateFixture.data as object),
      record_type: "lead",
    });
    expect(labels(root)).toEqual(["Not the same"]);
  });

  it("asks the assistant in chat when the host will not run tools for a view", async () => {
    proxies = false;
    const root = mount();
    expect(labels(root)).toEqual(["Ask the assistant to decide"]);
    await press(root, "Ask the assistant to decide");
    expect(calls).toHaveLength(0);
    expect(asked[0]).toContain("0195c3a0-0000-7000-8000-000000000002");
  });
});
