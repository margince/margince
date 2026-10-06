// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fieldConflictFixture } from "./fixture";

const calls: { tool: string; args: Record<string, unknown> }[] = [];
const asked: string[] = [];
let proxies = true;
const answers: Array<
  | { ok: true; data: unknown; warnings: [] }
  | { ok: false; reason: string; unknown?: true }
> = [];

vi.mock("../actions", () => ({
  declareActions: vi.fn(),
  canCallTools: () => proxies,
  callServerTool: async (tool: string, args: Record<string, unknown>) => {
    calls.push({ tool, args });
    return answers.shift() ?? { ok: true, data: {}, warnings: [] };
  },
  askAssistant: (text: string) => {
    asked.push(text);
  },
}));

const { render } = await import("./main");

const APPROVAL = "0195c3a0-0000-7000-8000-0000000000c1";

function mount(data: unknown = fieldConflictFixture.data): HTMLElement {
  const root = document.createElement("main");
  document.body.replaceChildren(root);
  render(root, data, []);
  return root;
}

const labels = (root: HTMLElement) =>
  [...root.querySelectorAll("button")].map((b) => b.textContent);

async function press(root: HTMLElement, label: string) {
  [...root.querySelectorAll("button")]
    .find((b) => b.textContent === label)
    ?.click();
  await new Promise((r) => setTimeout(r, 0));
}

beforeEach(() => {
  calls.length = 0;
  asked.length = 0;
  answers.length = 0;
  proxies = true;
});

describe("the field-conflict card", () => {
  it("draws nothing when the update held nothing back", () => {
    expect(
      mount({ record_type: "contact", id: "x", fields: {} }).childElementCount,
    ).toBe(0);
  });

  it("shows the value on the record beside the value proposed", () => {
    const root = mount();
    const row = root.querySelector("tbody tr");
    expect([...(row?.children ?? [])].map((n) => n.textContent)).toEqual([
      "job title",
      "Head of Sales",
      "VP Sales",
    ]);
  });

  it("keeps the current values by rejecting the staged change", async () => {
    const root = mount();
    await press(root, "Keep the current values");
    expect(calls).toEqual([
      {
        tool: "decide_approval",
        args: { staged_action_id: APPROVAL, decision: "reject" },
      },
    ]);
    expect(root.textContent).toContain("Kept the values on the record.");
  });

  it("uses the new values by approving, then redeeming the exact replay", async () => {
    const root = mount();
    await press(root, "Use the new values");
    expect(calls).toEqual([
      {
        tool: "decide_approval",
        args: { staged_action_id: APPROVAL, decision: "approve" },
      },
      {
        tool: "update_record",
        args: {
          record_type: "contact",
          id: "0195c3a0-0000-7000-8000-000000000003",
          fields: { job_title: "VP Sales" },
          approval_id: APPROVAL,
        },
      },
    ]);
    expect(root.textContent).toContain("Updated with the new values.");
  });

  it("offers only the finishing step when the approval landed and the update did not", async () => {
    answers.push(
      { ok: true, data: {}, warnings: [] },
      { ok: false, reason: "The record changed since." },
    );
    const root = mount();
    await press(root, "Use the new values");
    expect(labels(root)).toEqual(["Apply the new values"]);
    expect(root.querySelector(".refusal")?.textContent).toBe(
      "The record changed since.",
    );
    await press(root, "Apply the new values");
    expect(calls.filter((c) => c.tool === "decide_approval")).toHaveLength(1);
    expect(root.textContent).toContain("Updated with the new values.");
  });

  it("shows the engine's refusal of the approval and keeps both choices", async () => {
    answers.push({ ok: false, reason: "Already answered." });
    const root = mount();
    await press(root, "Use the new values");
    expect(root.querySelector(".refusal")?.textContent).toBe(
      "Already answered.",
    );
    expect(labels(root)).toEqual([
      "Keep the current values",
      "Use the new values",
    ]);
    expect(calls.map((c) => c.tool)).toEqual(["decide_approval"]);
  });

  it("asks the assistant in chat when the host will not run tools for a view", async () => {
    proxies = false;
    const root = mount();
    expect(labels(root)).toEqual(["Ask the assistant to decide"]);
    await press(root, "Ask the assistant to decide");
    expect(calls).toHaveLength(0);
    expect(asked[0]).toContain(APPROVAL);
  });

  it("offers the finishing step when the host never answered the approval", async () => {
    answers.push({
      ok: false,
      reason: "The host did not answer in time.",
      unknown: true,
    });
    const root = mount();
    await press(root, "Use the new values");
    expect(labels(root)).toEqual(["Apply the new values"]);
  });

  it("starts a different conflict drawn into the same frame undecided", async () => {
    const root = mount();
    await press(root, "Keep the current values");
    const other = JSON.parse(JSON.stringify(fieldConflictFixture.data));
    other.staged_approval.approval_id = "0195c3a0-0000-7000-8000-0000000000c2";
    render(root, other, []);
    expect(labels(root)).toEqual([
      "Keep the current values",
      "Use the new values",
    ]);
  });
});
