// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { createFollowupsFixture } from "./fixture";

const calls: { tool: string; args: Record<string, unknown> }[] = [];
const asked: string[] = [];
let proxies = true;
const answers: Array<
  | { ok: true; data: unknown; warnings: [] }
  | { ok: false; reason: string; unknown?: true }
> = [];
let answer:
  | { ok: true; data: unknown; warnings: [] }
  | { ok: false; reason: string; unknown?: true } = {
  ok: true,
  data: {},
  warnings: [],
};

vi.mock("../actions", () => ({
  declareActions: vi.fn(),
  canCallTools: () => proxies,
  callServerTool: async (tool: string, args: Record<string, unknown>) => {
    calls.push({ tool, args });
    return answers.shift() ?? answer;
  },
  askAssistant: (text: string) => {
    asked.push(text);
  },
}));

const { render } = await import("./main");

function mount(data: unknown = createFollowupsFixture.data): HTMLElement {
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
  answers.length = 0;
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
    render(root, createFollowupsFixture.data, []);
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
      ...(createFollowupsFixture.data as object),
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

describe("the tag offer", () => {
  const withOffer = (offer: object, recordType = "contact") => ({
    ...(createFollowupsFixture.data as object),
    record_type: recordType,
    duplicate_candidates: [],
    tag_offer: offer,
  });
  const existing = {
    name: "K5 Conference 2026",
    tag_id: "0195c3a0-0000-7000-8000-0000000000e1",
    exists: true,
    may_create: false,
  };

  it("draws no panel when the create carried no offer", () => {
    const root = mount({ record_type: "contact", id: "x", fields: {} });
    expect(root.childElementCount).toBe(0);
  });

  it("applies an existing word by its id, and offers an undo that takes it off", async () => {
    const root = mount(withOffer(existing));
    expect(labels(root)).toEqual(["Tag it", "No thanks"]);
    await press(root, "Tag it");
    expect(calls).toEqual([
      {
        tool: "apply_tag",
        args: {
          record_type: "contact",
          record_id: "0195c3a0-0000-7000-8000-000000000002",
          tag_id: existing.tag_id,
        },
      },
    ]);
    expect(root.textContent).toContain("Tagged “K5 Conference 2026”.");
    await press(root, "Undo");
    expect(calls[1].tool).toBe("remove_tag");
    expect(root.textContent).toContain("Not tagged.");
  });

  it("coins a new word first when the seat may, and applies it by the id it was given", async () => {
    answers.push({ ok: true, data: { tag_id: "t9" }, warnings: [] });
    const root = mount(
      withOffer({ name: "Fair", exists: false, may_create: true }),
    );
    expect(labels(root)).toEqual(["Add the tag and tag it", "No thanks"]);
    await press(root, "Add the tag and tag it");
    expect(calls.map((c) => c.tool)).toEqual(["create_tag", "apply_tag"]);
    expect(calls[0].args).toEqual({ name: "Fair" });
    expect(calls[1].args.tag_id).toBe("t9");
  });

  it("retries a failed apply without coining the word a second time", async () => {
    answers.push(
      { ok: true, data: { tag_id: "t9" }, warnings: [] },
      { ok: false, reason: "The record is archived." },
    );
    const root = mount(
      withOffer({ name: "Fair", exists: false, may_create: true }),
    );
    await press(root, "Add the tag and tag it");
    expect(labels(root)).toEqual(["Tag it", "No thanks"]);
    await press(root, "Tag it");
    expect(calls.map((c) => c.tool)).toEqual([
      "create_tag",
      "apply_tag",
      "apply_tag",
    ]);
    expect(root.textContent).toContain("Tagged “Fair”.");
  });

  it("keeps Undo when taking the tag off failed, since it may still be on", async () => {
    const root = mount(withOffer(existing));
    await press(root, "Tag it");
    answers.push({ ok: false, reason: "The record is read-only." });
    await press(root, "Undo");
    expect(labels(root)).toEqual(["Undo"]);
    expect(root.querySelector(".refusal")?.textContent).toBe(
      "The record is read-only.",
    );
  });

  it("offers no button for a word the seat cannot add", () => {
    const root = mount(
      withOffer({ name: "Fair", exists: false, may_create: false }),
    );
    expect(labels(root)).toEqual([]);
    expect(root.querySelector(".refusal")?.textContent).toContain(
      "cannot add one",
    );
  });

  it("declining makes no call", async () => {
    const root = mount(withOffer(existing));
    await press(root, "No thanks");
    expect(calls).toHaveLength(0);
    expect(root.textContent).toContain("Not tagged.");
  });

  it("says why when the tag could not be applied", async () => {
    answer = { ok: false, reason: "That tag was archived." };
    const root = mount(withOffer(existing));
    await press(root, "Tag it");
    expect(root.querySelector(".refusal")?.textContent).toBe(
      "That tag was archived.",
    );
    expect(labels(root)).toEqual(["Tag it", "No thanks"]);
  });

  it("is not drawn for a record type a tag cannot sit on", () => {
    expect(mount(withOffer(existing, "activity")).childElementCount).toBe(0);
  });
});

describe("the tag offer without host tool calls", () => {
  it("asks the assistant in chat rather than drawing a dead panel", async () => {
    proxies = false;
    const root = mount({
      record_type: "contact",
      id: "0195c3a0-0000-7000-8000-000000000002",
      fields: {},
      tag_offer: {
        name: "Fair",
        exists: true,
        tag_id: "t1",
        may_create: false,
      },
    });
    expect(labels(root)).toEqual(["Ask the assistant to tag it"]);
    await press(root, "Ask the assistant to tag it");
    expect(calls).toHaveLength(0);
    expect(asked[0]).toContain("Fair");
  });
});

describe("a host call that never answered", () => {
  it("offers no retry, because the merge may have landed", async () => {
    answer = {
      ok: false,
      reason:
        "The host did not answer in time, so the change may have gone through.",
      unknown: true,
    };
    const root = mount();
    await press(root, "Merge into the existing record");
    expect(labels(root)).toEqual([]);
    expect(root.textContent).toContain("may have gone through");
  });
});

describe("a create that filed more than one pair", () => {
  it("settles every pair once the created record is merged", async () => {
    const data = createFollowupsFixture.data as {
      duplicate_candidates: Array<Record<string, unknown>>;
    };
    const second = {
      ...data.duplicate_candidates[0],
      candidate_id: "0195c3a0-0000-7000-8000-0000000000ab",
      other_record_id: "0195c3a0-0000-7000-8000-000000000009",
    };
    const root = mount({
      ...data,
      duplicate_candidates: [...data.duplicate_candidates, second],
    });
    await press(root, "Merge into the existing record");
    expect(labels(root)).toEqual([]);
    expect(calls.filter((c) => c.tool === "merge_records")).toHaveLength(1);
  });
});
