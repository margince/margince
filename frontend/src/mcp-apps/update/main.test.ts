// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { expectReadable } from "../readable.testkit";
import { updateFixture } from "./fixture";
import { render } from "./main";

const LEAD = "0195c3a0-0000-7000-8000-0000000000a1";
const REP = "0195c3a0-0000-7000-8000-0000000000a2";

function updated(
  fields: Record<string, unknown>,
  changes: unknown[],
): HTMLElement {
  const root = document.createElement("main");
  document.body.replaceChildren(root);
  render(root, { record_type: "lead", id: LEAD, fields, changes }, []);
  return root;
}

const rows = (root: HTMLElement) =>
  [...root.querySelectorAll("tbody tr")].map((r) => r.textContent);

describe("the update card", () => {
  it("says the update is saved when it lists no change", () => {
    const root = document.createElement("main");
    render(root, { record_type: "contact", id: "x", fields: {} }, []);
    expect(root.querySelector("h1")?.textContent).toBe("Updated a contact");
    expect(root.querySelector(".intro")?.textContent).toBe(
      "The change is saved in Margince.",
    );
  });

  it("names the record and the owner it handed over, before and after", () => {
    const root = updated({ full_name: "Jane Doe", owner_id: REP }, [
      {
        field: "owner_id",
        label: "Owner",
        before: { kind: "owner", text: "Ana Rep" },
        after: { kind: "owner", text: "Sam Rep" },
      },
    ]);
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe(
      'Updated lead "Jane Doe"',
    );
    expect(rows(root)).toEqual(["OwnerAna Rep → Sam Rep"]);
  });

  it("shows an amount in its currency and an empty value as a dash", () => {
    const root = updated({ full_name: "Jane Doe" }, [
      {
        field: "amount_minor",
        label: "Value",
        before: { kind: "empty", text: "" },
        after: { kind: "money", text: "120000", currency: "EUR" },
      },
    ]);
    expectReadable(root);
    expect(rows(root)[0]).toMatch(/^Value— → .*1,?200/);
  });

  it("draws the sealed answer the tool sends", () => {
    const root = document.createElement("main");
    render(root, updateFixture.data, []);
    expectReadable(root);
    expect(rows(root)).toEqual(["Job titleHead of Sales → VP Sales"]);
  });

  it("says nothing was changed when the update was refused", () => {
    const root = document.createElement("main");
    render(root, null, [], { refusal: "A lead cannot be its own parent." });
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe("Nothing was changed");
    expect(root.querySelector(".intro")?.textContent).toBe(
      "A lead cannot be its own parent.",
    );
  });
});
