// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { expectReadable } from "../readable.testkit";
import { approvalQueueFixture } from "./fixture";

const calls: { tool: string; args: Record<string, unknown> }[] = [];
const asked: string[] = [];
let proxies = true;

vi.mock("../actions", () => ({
  declareActions: vi.fn(),
  canCallTools: () => proxies,
  callServerTool: async (tool: string, args: Record<string, unknown>) => {
    calls.push({ tool, args });
    return { ok: true, data: {}, warnings: [] };
  },
  askAssistant: (text: string) => {
    asked.push(text);
  },
}));

const { render } = await import("./main");

function mount(data: unknown = approvalQueueFixture.data): HTMLElement {
  const root = document.createElement("main");
  document.body.replaceChildren(root);
  render(root, data, []);
  return root;
}

const labels = (root: HTMLElement) =>
  [...root.querySelectorAll("button")].map((b) => b.textContent);

async function press(root: HTMLElement, label: string, nth = 0) {
  [...root.querySelectorAll("button")]
    .filter((b) => b.textContent === label)
    [nth]?.click();
  await new Promise((r) => setTimeout(r, 0));
}

const FIRST = "0195c3a0-0000-7000-8000-0000000000b1";
const BUNDLE = "0195c3a0-0000-7000-8000-0000000000c1";

beforeEach(() => {
  calls.length = 0;
  asked.length = 0;
  proxies = true;
});

describe("the approval queue", () => {
  it("lists every proposal with its own answers, and counts what waits", () => {
    const root = mount();
    expectReadable(root);
    expect(root.querySelectorAll(".dcard")).toHaveLength(3);
    expect(root.querySelector(".panel-head")?.textContent).toContain("3");
    expect(labels(root).filter((l) => l === "Approve")).toHaveLength(3);
  });

  it("says nothing is waiting for an empty page", () => {
    const root = mount({ approvals: [] });
    expectReadable(root);
    expect(root.querySelector(".intro")?.textContent).toBe(
      "Nothing is waiting for a decision.",
    );
    expect(labels(root)).toEqual([]);
  });

  it("answers one proposal by its id and hands the go-ahead back", async () => {
    const root = mount();
    await press(root, "Approve");
    expect(calls).toEqual([
      {
        tool: "decide_approvals",
        args: { staged_action_id: FIRST, decision: "approve" },
      },
    ]);
    expect(asked[0]).toContain(`approval_id ${FIRST}`);
    expect(labels(root).filter((l) => l === "Approve")).toHaveLength(2);
  });

  it("answers a bundle's waiting members together", async () => {
    const root = mount();
    await press(root, "Approve all 2");
    expect(calls).toEqual([
      {
        tool: "decide_approvals",
        args: { bundle_id: BUNDLE, decision: "approve" },
      },
    ]);
    expect(root.querySelectorAll(".dcard .settled")).toHaveLength(2);
    expect(labels(root)).not.toContain("Approve all 2");
  });

  it("offers no bundle answer when the host will not run tools", () => {
    proxies = false;
    const root = mount();
    expect(labels(root)).not.toContain("Approve all 2");
    expect(
      labels(root).filter((l) => l === "Ask the assistant to decide"),
    ).toHaveLength(3);
  });

  it("asks the assistant for the next page when there is one", async () => {
    const root = mount({
      ...(approvalQueueFixture.data as object),
      next_cursor: "page-2",
    });
    await press(root, "Show the next page");
    expect(calls).toEqual([]);
    expect(asked[0]).toContain("page-2");
  });

  it("offers no answer on a proposal already answered", () => {
    const root = mount({
      approvals: [
        {
          staged_action_id: FIRST,
          kind: "advance_deal",
          status: "rejected",
          summary: "Move Acme renewal to Negotiation",
          proposed_by: "agent:claude",
        },
      ],
    });
    expect(labels(root)).toEqual([]);
    expect(root.querySelector(".dcard-meta")?.textContent).toContain(
      "Rejected",
    );
  });

  it("says the queue could not be read when the call was refused", () => {
    const root = document.createElement("main");
    render(root, null, [], { refusal: "The read scope is missing." });
    expect(root.querySelector("h1")?.textContent).toBe(
      "Could not read the queue",
    );
  });
});
