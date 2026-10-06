// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
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
  it("draws nothing for an answer that is not a staged action", () => {
    expect(mount({}).childElementCount).toBe(0);
  });

  it("shows what is proposed and by whom", () => {
    const root = mount();
    expect(root.querySelector(".name")?.textContent).toBe(
      "Move Acme renewal to Negotiation",
    );
    expect(root.textContent).toContain("Proposed by agent:claude");
    expect(
      [...root.querySelectorAll("tbody tr")].map((r) => r.textContent),
    ).toEqual(["stageNegotiation", "amount minor1200000"]);
  });

  it("approves through decide_approval and says the assistant still has to run it", async () => {
    const root = mount();
    await press(root, "Approve");
    expect(calls).toEqual([
      {
        tool: "decide_approval",
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
