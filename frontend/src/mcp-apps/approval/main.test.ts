// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { expectReadable } from "../readable.testkit";
import { approvalFixture } from "./fixture";

const calls: { tool: string; args: Record<string, unknown> }[] = [];
const asked: string[] = [];
let proxies = true;
let answer:
  | { ok: true; data: unknown; warnings: [] }
  | { ok: false; reason: string } = { ok: true, data: {}, warnings: [] };

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

function mount(data: unknown = approvalFixture.data): HTMLElement {
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

const ID = "0195c3a0-0000-7000-8000-0000000000b1";

beforeEach(() => {
  calls.length = 0;
  asked.length = 0;
  proxies = true;
  answer = { ok: true, data: {}, warnings: [] };
});

describe("the approval card", () => {
  it("says there is nothing to decide for an answer that is not a staged action", () => {
    const root = mount({});
    expect(root.querySelector("h1")?.textContent).toBe("No approval to show");
    expect(labels(root)).toEqual([]);
  });

  it("shows what is proposed and by whom", () => {
    const root = mount();
    expect(root.querySelector(".name")?.textContent).toBe(
      "Move Acme renewal to Negotiation",
    );
    expect(root.textContent).toContain("Proposed by claude");
    expect(
      [...root.querySelectorAll("tbody tr")].map((r) => r.textContent),
    ).toEqual(["StageNegotiation", "Value—"]);
  });

  it("shows a nested change as words rather than hiding it", () => {
    const root = mount({
      ...(approvalFixture.data as object),
      proposed_change: { body: { text: "Hello Anna" }, to: ["a@b.test"] },
    });
    const cells = [...root.querySelectorAll("tbody td")].map(
      (n) => n.textContent,
    );
    expect(cells).toEqual(["Text: Hello Anna", "a@b.test"]);
  });

  it("shows the evidence the proposal was read out of", () => {
    const root = mount({
      ...(approvalFixture.data as object),
      evidence: [{ evidence_snippet: "Anna asked to move to Negotiation." }],
    });
    expect(root.querySelector(".evidence-list")?.textContent).toBe(
      "Anna asked to move to Negotiation.",
    );
  });

  it("shows a long nested change in full", () => {
    const long = "x".repeat(900);
    const root = mount({
      ...(approvalFixture.data as object),
      proposed_change: { body: { text: long } },
    });
    expect(root.querySelector("tbody td")?.textContent).toContain(long);
  });

  it("approves through decide_approvals and says the assistant still has to run it", async () => {
    const root = mount();
    await press(root, "Approve");
    expect(calls).toEqual([
      {
        tool: "decide_approvals",
        args: { staged_action_id: ID, decision: "approve" },
      },
    ]);
    expect(root.textContent).toContain(
      "Approved. The assistant still has to run it again.",
    );
    expect(labels(root)).toEqual([]);
  });

  it("rejects, and the settled line survives a redelivered result", async () => {
    const root = mount();
    await press(root, "Reject");
    expect(calls[0].args.decision).toBe("reject");
    render(root, approvalFixture.data, []);
    expect(root.textContent).toContain("Rejected. Nothing was changed.");
  });

  it("shows the engine's refusal and keeps both choices", async () => {
    answer = { ok: false, reason: "Only the owner can release a send." };
    const root = mount();
    await press(root, "Approve");
    expect(root.querySelector(".refusal")?.textContent).toBe(
      "Only the owner can release a send.",
    );
    expect(labels(root)).toEqual(["Approve", "Reject"]);
  });

  it("offers no choice for an item already answered", () => {
    const root = mount({
      ...(approvalFixture.data as object),
      status: "approved",
    });
    expect(labels(root)).toEqual([]);
    expect(root.textContent).toContain("Already approved.");
  });

  it("offers no choice for a lapsed item", () => {
    const root = mount({
      ...(approvalFixture.data as object),
      expires_at: "2020-01-01T00:00:00Z",
    });
    expect(labels(root)).toEqual([]);
    expect(root.textContent).toContain("Expired");
  });

  it("asks the assistant in chat when the host will not run tools for a view", async () => {
    proxies = false;
    const root = mount();
    expect(labels(root)).toEqual(["Ask the assistant to decide"]);
    await press(root, "Ask the assistant to decide");
    expect(calls).toHaveLength(0);
    expect(asked[0]).toContain(ID);
  });
});

describe("an approval for a held update", () => {
  it("shows the fields it would set, not the call that carries them", () => {
    const record = "0195c3a0-0000-7000-8000-0000000000a1";
    const root = mount({
      ...(approvalFixture.data as object),
      summary: 'Update lead "Jane Doe": overwrite Job title, edited by hand',
      proposed_by: "agent:0195c3a0-0000-7000-8000-0000000000a9",
      proposed_change: {
        record_type: "lead",
        id: record,
        fields: { title: "VP Sales" },
      },
    });
    expectReadable(root);
    expect(root.textContent).toContain("Proposed by an assistant");
    expect(
      [...root.querySelectorAll("tbody tr")].map((r) => r.textContent),
    ).toEqual(["Job titleVP Sales"]);
  });
});
