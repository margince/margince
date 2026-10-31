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

  it("shows what is proposed, by whom and for whom", () => {
    const root = mount();
    expect(root.querySelector("h1")?.textContent).toBe(
      "Move Acme renewal to Negotiation",
    );
    expect(root.textContent).toContain("Proposed by an assistant for Sam Rep");
    expect(
      [...root.querySelectorAll("tbody tr")].map((r) => r.textContent),
    ).toEqual(["StageProposal → Negotiation"]);
  });

  it("shows a structured value as words rather than as its document", () => {
    const root = mount({
      ...(approvalFixture.data as object),
      changes: [
        {
          field: "address",
          label: "Address",
          before: { kind: "empty", text: "" },
          after: { kind: "structured", text: '{"city":"Berlin","zip":""}' },
        },
      ],
    });
    expectReadable(root);
    expect(root.querySelector("tbody td")?.textContent).toBe(
      "— → City: Berlin",
    );
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

  it("shows a long value in full", () => {
    const long = "x".repeat(900);
    const root = mount({
      ...(approvalFixture.data as object),
      changes: [
        {
          field: "description",
          label: "Description",
          before: { kind: "empty", text: "" },
          after: { kind: "text", text: long },
        },
      ],
    });
    expect(root.querySelector("tbody td")?.textContent).toContain(long);
  });

  it("names nothing it was not told, for a proposal it cannot preview", () => {
    const root = mount({ ...(approvalFixture.data as object), changes: [] });
    expectReadable(root);
    expect(root.querySelector("tbody")).toBeNull();
  });

  it("shows the operands and further effects of a ready preview", () => {
    const root = mount({
      ...(approvalFixture.data as object),
      changes: [],
      preview_state: "ready",
      details: [
        {
          field: "promotion_contact_name",
          label: "Promotion contact name",
          value: { kind: "text", text: "Pia Parity" },
        },
      ],
      effects: [
        {
          effect: "promote_lead_archive_source",
          label: "Qualifies and archives this lead.",
        },
      ],
    });
    expectReadable(root);
    expect(
      [...root.querySelectorAll("tbody tr")].map((r) => r.textContent),
    ).toEqual(["Promotion contact namePia Parity"]);
    expect(root.querySelector(".dcard-body")?.textContent).toContain(
      "Also changesQualifies and archives this lead.",
    );
    expect(labels(root)).toEqual(["Reject", "Approve"]);
  });

  it("offers no approval for a preview the reader cannot see, and says who decides", () => {
    const note =
      "This change touches records you cannot open. A colleague who can see them must decide it.";
    const root = mount({
      ...(approvalFixture.data as object),
      changes: [],
      preview_state: "needs_wider_sight",
      preview_note: note,
    });
    expect(labels(root)).toEqual([]);
    expect(root.querySelector(".dcard-verbs")?.textContent).toBe(note);
    expect(root.querySelector("tbody")).toBeNull();
  });

  it("approves through decide_approvals and asks the assistant to run it", async () => {
    const root = mount();
    await press(root, "Approve");
    expect(calls).toEqual([
      {
        tool: "decide_approvals",
        args: { staged_action_id: ID, decision: "approve" },
      },
    ]);
    expect(root.textContent).toContain(
      "Approved. The assistant is running it now.",
    );
    expect(labels(root)).toEqual([]);
    expect(asked).toHaveLength(1);
    expect(asked[0]).toContain(`approval_id ${ID}`);
  });

  it("asks nothing of the assistant once a colleague's proposal is approved", async () => {
    const root = mount({
      ...(approvalFixture.data as object),
      proposed_by: "human:0195c3a0-0000-7000-8000-0000000000a3",
    });
    await press(root, "Approve");
    expect(root.textContent).toContain("Approved.");
    expect(asked).toEqual([]);
  });

  it("asks nothing of the assistant after a rejection", async () => {
    const root = mount();
    await press(root, "Reject");
    expect(asked).toEqual([]);
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
    expect(labels(root)).toEqual(["Reject", "Approve"]);
  });

  it("offers no choice for an item already answered", () => {
    const root = mount({
      ...(approvalFixture.data as object),
      status: "approved",
    });
    expect(labels(root)).toEqual([]);
    expect(root.querySelector(".dcard-meta")?.textContent).toContain(
      "Approved",
    );
  });

  it("offers no choice for a lapsed item", () => {
    const root = mount({
      ...(approvalFixture.data as object),
      expires_at: "2020-01-01T00:00:00Z",
    });
    expect(labels(root)).toEqual([]);
    expect(root.textContent).toContain("ran out of time");
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
      on_behalf_of: { id: record, is_you: true },
      changes: [
        {
          field: "title",
          label: "Job title",
          before: { kind: "text", text: "Head of Sales" },
          after: { kind: "text", text: "VP Sales" },
        },
      ],
    });
    expectReadable(root);
    expect(root.textContent).toContain("Proposed by an assistant for you");
    expect(
      [...root.querySelectorAll("tbody tr")].map((r) => r.textContent),
    ).toEqual(["Job titleHead of Sales → VP Sales"]);
  });
});
