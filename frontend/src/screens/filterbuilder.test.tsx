/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { FilterBuilder } from "./filterbuilder";
import type { VocabularyField } from "./filterdata";
import {
  encode,
  isGroup,
  type Node,
  newGroup,
  newLeaf,
  resetIDsForTest,
} from "./segmentpredicate";

// The builder's one job is that every choice it offers came from the server's
// vocabulary. So the tests below mostly ask what it OFFERS, not what it draws:
// an operator in a list the field's type does not admit is a clause the engine
// will refuse, and that is the failure this screen exists to prevent.

// A new clause starts on owner_id, whose picker reads the seat roster, so
// every test owns its fetch: none reaches the network or a neighbour's stub.
beforeEach(stubSeats);
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const VOCAB: VocabularyField[] = [
  {
    name: "owner_id",
    type: "id",
    operators: ["eq", "neq", "in", "exists"],
    custom: false,
    // The vocabulary names an id field's target, so the fixture does too — a
    // stub that omitted it would exercise a response the server cannot send.
    references: "app_user",
  },
  {
    // An unbounded target: too many accounts to enumerate, so this one keeps the
    // plain box until the async picker exists.
    name: "company_id",
    type: "id",
    operators: ["eq", "neq", "in", "exists"],
    custom: false,
    references: "company",
  },
  {
    name: "full_name",
    type: "text",
    operators: ["eq", "neq", "in", "contains", "exists"],
    custom: false,
  },
  {
    name: "created_at",
    type: "date",
    operators: ["eq", "neq", "gt", "gte", "lt", "lte", "exists"],
    custom: false,
  },
  {
    name: "cf_loyalty_tier",
    type: "picklist",
    operators: ["eq", "neq", "in", "exists"],
    custom: true,
    // The vocabulary carries a picklist's allowed values, so the fixture does —
    // a stub without them would exercise a response the server cannot send.
    options: ["gold", "silver", "bronze"],
  },
  {
    name: "cf_deal_score",
    type: "number",
    operators: ["eq", "neq", "gt", "gte", "lt", "lte", "in", "exists"],
    custom: true,
  },
];

/** Controlled, because a builder that never receives its own edits back proves
 *  only that a callback fired. */
function Harness({ start }: Readonly<{ start: Node }>) {
  const [tree, setTree] = useState<Node>(start);
  // A fresh client per mount: the record pickers read rosters, and a shared
  // cache would let one test's options answer another's assertion.
  const [client] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );
  return (
    <QueryClientProvider client={client}>
      <FilterBuilder tree={tree} onChange={setTree} fields={VOCAB} />
      {/* The encoded tree is the thing the server would receive, so the test
          asserts against that rather than against the DOM's rendering of it. */}
      <pre data-testid="wire">{JSON.stringify(encode(tree))}</pre>
      {/* The proposed marks, which the wire never carries: one per clause, in
          reading order. */}
      <pre data-testid="marks">{JSON.stringify(marksOf(tree))}</pre>
    </QueryClientProvider>
  );
}

/** The seats a record picker offers. Named, so an assertion reads as a contact. */
function stubSeats() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      const body = url.includes("/users")
        ? {
            data: [
              { id: "u-1", display_name: "Ann Lee" },
              { id: "u-2", display_name: "Bruno Sá" },
            ],
            page: { next_cursor: null, has_more: false },
          }
        : { data: [], page: { next_cursor: null, has_more: false } };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

// The companies door as the search calls it, plus the seats every other picker
// on this screen reads on mount.
function stubCompanies() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      let body: unknown = {
        data: [],
        page: { next_cursor: null, has_more: false },
      };
      if (url.includes("/companies")) {
        // Answers only what the query narrows to, so a test asking for
        // something absent gets the empty answer rather than a stub that
        // always has a hit.
        const q = new URL(url, "http://test").searchParams.get("q") ?? "";
        body = {
          data:
            "northgate".includes(q.toLowerCase()) && q !== ""
              ? [{ id: "company-1", display_name: "Northgate" }]
              : [],
          page: { next_cursor: null, has_more: false },
        };
      }
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function wire() {
  return JSON.parse(screen.getByTestId("wire").textContent ?? "{}");
}

function marksOf(node: Node): boolean[] {
  return isGroup(node)
    ? node.children.flatMap(marksOf)
    : [node.proposed === true];
}

function marks(): boolean[] {
  return JSON.parse(screen.getByTestId("marks").textContent ?? "[]");
}

/** A clause a model proposed and the reader has not touched. */
function proposedLeaf(
  field: string,
  op: Parameters<typeof newLeaf>[1],
  value: Parameters<typeof newLeaf>[2],
): Node {
  return { ...newLeaf(field, op, value), proposed: true };
}

describe("what the builder offers", () => {
  it("offers a field's operators and no others", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness start={newGroup("and", [newLeaf("created_at", "gt", "")])} />,
    );

    await user.click(screen.getByRole("combobox", { name: "Operator" }));

    // A date admits ordering and equality; it does NOT admit contains or in, and
    // offering either would build a clause the engine refuses.
    expect(screen.getByRole("option", { name: "is on or after" })).toBeTruthy();
    expect(screen.queryByRole("option", { name: "contains" })).toBeNull();
    expect(screen.queryByRole("option", { name: "is any of" })).toBeNull();
  });

  it("reads an ordering operator as a quantity on a number and a date on a date", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [newLeaf("cf_deal_score", "gte", "")])}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Operator" }));

    // The same operator, read the way its field's type means it. "is on or after"
    // on a score would send a reader looking for a calendar.
    expect(screen.getByRole("option", { name: "is at least" })).toBeTruthy();
    expect(screen.queryByRole("option", { name: "is on or after" })).toBeNull();
  });

  it("marks a workspace-defined field as one", () => {
    resetIDsForTest();
    render(
      <Harness
        start={newGroup("and", [newLeaf("cf_loyalty_tier", "eq", "gold")])}
      />,
    );

    // The badge is what tells a reader this column is theirs rather than the
    // product's — AC-filters-and-views-3 asks for it by name. Neutral: emerald
    // is the page's one primary action.
    const badge = screen.getByText("Custom field").closest(".badge");
    expect(badge?.className).not.toContain("badge-accent");
    expect(screen.queryByText("owner id")).toBeNull();
  });

  it("does not mark a core field as custom", () => {
    resetIDsForTest();
    render(
      <Harness start={newGroup("and", [newLeaf("owner_id", "eq", "u1")])} />,
    );

    expect(screen.queryByText("Custom field")).toBeNull();
  });
});

describe("an id clause names a record, not a uuid", () => {
  it("offers the records the vocabulary's target points at", async () => {
    resetIDsForTest();
    stubSeats();
    const user = userEvent.setup();
    render(
      <Harness start={newGroup("and", [newLeaf("owner_id", "eq", "")])} />,
    );

    // The seat is chosen by NAME. Before this, the same clause needed a uuid
    // typed into a text box, and a typo read as a filter matching nothing.
    await pickOption(
      user,
      await screen.findByRole("combobox", { name: "Value" }),
      "Bruno Sá",
    );

    // And the wire still carries the id, which is what the engine compares.
    expect(wire()).toEqual({
      and: [{ field: "owner_id", op: "eq", value: "u-2" }],
    });
  });

  it("searches for a target too large to enumerate, and only a hit becomes the value", async () => {
    resetIDsForTest();
    stubCompanies();
    const user = userEvent.setup();
    render(
      <Harness start={newGroup("and", [newLeaf("company_id", "eq", "")])} />,
    );

    // An account list grows with the business, so it is not a dropdown. It is
    // not a uuid box either: nobody types one from memory, and one typed wrong
    // matches nothing and reads as "no companies match".
    const box = await screen.findByRole("textbox", {
      name: "Search companies",
    });
    expect(screen.queryByRole("combobox", { name: "Value" })).toBeNull();

    // The TYPED WORDS ARE NOT THE VALUE. This is the whole guarantee: until a
    // hit is chosen the clause carries nothing, so a half-typed name cannot
    // reach the engine as one.
    await user.type(box, "north");
    expect(wire()).toEqual({
      and: [{ field: "company_id", op: "eq", value: "" }],
    });

    await user.click(await screen.findByRole("button", { name: "Northgate" }));

    // And what lands on the wire is the id, chosen rather than composed.
    expect(wire()).toEqual({
      and: [{ field: "company_id", op: "eq", value: "company-1" }],
    });
    // The name stands where the search box was, so the reader can see their
    // choice took rather than wondering whether it did.
    expect(screen.getByText("Northgate")).toBeTruthy();
  });

  it("says a search found nothing rather than showing an empty list", async () => {
    resetIDsForTest();
    stubCompanies();
    const user = userEvent.setup();
    render(
      <Harness start={newGroup("and", [newLeaf("company_id", "eq", "")])} />,
    );

    // A list with no rows and no line above it reads as a confident "this
    // workspace has no such company", which is a settled answer to a question
    // that got one — the exact failure this whole surface exists to prevent.
    await user.type(
      await screen.findByRole("textbox", { name: "Search companies" }),
      "zzz",
    );
    expect(await screen.findByText("No companies match")).toBeTruthy();
  });

  it("names records one at a time for a list clause, and never as free text", async () => {
    resetIDsForTest();
    stubSeats();
    const user = userEvent.setup();
    render(
      <Harness start={newGroup("and", [newLeaf("owner_id", "in", [])])} />,
    );

    // `in` used to win over the reference and hand back a token box, so every
    // id field's LIST was free text — a uuid typed wrong there compiles,
    // matches nothing, and reads as a settled "no rows" exactly as the single
    // case did. The operator changes how many records are named, not whether
    // they are chosen.
    expect(screen.queryByRole("textbox", { name: "Values" })).toBeNull();

    await pickOption(
      user,
      await screen.findByRole("combobox", { name: "Value" }),
      "Ann Lee",
    );
    await pickOption(
      user,
      await screen.findByRole("combobox", { name: "Value" }),
      "Bruno Sá",
    );

    expect(wire()).toEqual({
      and: [{ field: "owner_id", op: "in", value: ["u-1", "u-2"] }],
    });

    // And a record already named can be dropped, or a list is a one-way door.
    await user.click(screen.getByRole("button", { name: "Remove Ann Lee" }));
    expect(wire()).toEqual({
      and: [{ field: "owner_id", op: "in", value: ["u-2"] }],
    });
  });

  it("searches for each company a list clause names", async () => {
    resetIDsForTest();
    stubCompanies();
    const user = userEvent.setup();
    render(
      <Harness start={newGroup("and", [newLeaf("company_id", "in", [])])} />,
    );

    // The unbounded target takes the same rule through its own control: the
    // search box stays open under what the clause already holds, because the
    // next pick is the point.
    await user.type(
      await screen.findByRole("textbox", { name: "Search companies" }),
      "north",
    );
    await user.click(await screen.findByRole("button", { name: "Northgate" }));

    expect(wire()).toEqual({
      and: [{ field: "company_id", op: "in", value: ["company-1"] }],
    });
    expect(
      screen.getByRole("textbox", { name: "Search companies" }),
    ).toBeTruthy();
  });

  it("asks nothing of a reader when the operator already answered", async () => {
    resetIDsForTest();
    render(
      <Harness
        start={newGroup("and", [newLeaf("owner_id", "exists", true)])}
      />,
    );

    // `exists` on an id field is a two-way question the operator itself asked, so
    // offering a record to compare against would ask for an operand the engine
    // ignores. The operator arms come first in the control for that reason.
    expect(screen.queryByRole("combobox", { name: "Value" })).toBeNull();
    expect(screen.getByRole("button", { name: "has a value" })).toBeTruthy();
  });
});

describe("when a roster cannot be read", () => {
  it("falls back to a box so the clause can still be written", async () => {
    resetIDsForTest();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ title: "Unavailable", status: 503 }), {
            status: 503,
            headers: { "Content-Type": "application/problem+json" },
          }),
      ),
    );
    render(
      <Harness start={newGroup("and", [newLeaf("owner_id", "eq", "")])} />,
    );

    // Not an empty dropdown: that would claim this workspace has no seats, and
    // would leave the reader unable to write the clause at all.
    expect(await screen.findByRole("textbox", { name: "Value" })).toBeTruthy();
    expect(screen.queryByRole("combobox", { name: "Value" })).toBeNull();
  });
});

describe("a closed set is picked, not typed", () => {
  it("offers the values the vocabulary carries", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [newLeaf("cf_loyalty_tier", "eq", "")])}
      />,
    );

    await pickOption(
      user,
      await screen.findByRole("combobox", { name: "Value" }),
      "silver",
    );

    expect(wire()).toEqual({
      and: [{ field: "cf_loyalty_tier", op: "eq", value: "silver" }],
    });
  });

  it("gives a reader no way to compose a value the set does not hold", async () => {
    resetIDsForTest();
    render(
      <Harness
        start={newGroup("and", [newLeaf("cf_loyalty_tier", "eq", "")])}
      />,
    );

    // No text box at all for a closed set. That is the whole fix: a free box
    // over these values let `Gold` through, which compiled, matched nothing, and
    // reported "0 match" as a settled answer.
    expect(screen.queryByRole("textbox", { name: "Value" })).toBeNull();
    const listed = (await screen.findByRole("combobox", { name: "Value" }))
      .textContent;
    expect(listed).not.toContain("Gold");
  });

  it("still types a free-text field", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness start={newGroup("and", [newLeaf("full_name", "eq", "")])} />,
    );

    // The other half of the rule: `text` has no closed set, so a box is right
    // there and turning every field into a dropdown would be the opposite error.
    await user.type(
      await screen.findByRole("textbox", { name: "Value" }),
      "ann",
    );

    expect(wire()).toEqual({
      and: [{ field: "full_name", op: "eq", value: "ann" }],
    });
  });
});

describe("editing the tree", () => {
  it("adds a condition to the group whose button was pressed", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(<Harness start={newGroup("and", [])} />);

    await user.click(screen.getByRole("button", { name: "Add condition" }));

    // The first field the picker would offer, with its first admitted operator.
    expect(wire()).toEqual({
      and: [{ field: "owner_id", op: "eq", value: "" }],
    });
  });

  it("removes the condition whose control was pressed, not the last one", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [
          newLeaf("owner_id", "eq", "u1"),
          newLeaf("full_name", "contains", "ann"),
        ])}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Remove Owner condition" }),
    );

    expect(wire()).toEqual({
      and: [{ field: "full_name", op: "contains", value: "ann" }],
    });
  });

  it("drops an operator the new field cannot take when the field changes", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [newLeaf("full_name", "contains", "ann")])}
      />,
    );

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Field" }),
      "Created",
    );

    // A date has no `contains`, so the clause falls back to the new field's first
    // admitted operator rather than keeping one that would be refused.
    const after = wire();
    expect(after.and[0].field).toBe("created_at");
    expect(after.and[0].op).toBe("eq");
    expect(after.and[0].value).toBe("");
  });

  it("keeps half-typed numeric input rather than coercing it", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [newLeaf("cf_deal_score", "gte", "")])}
      />,
    );

    // "-" is neither a number nor a mistake; coercing it would either move the
    // count or refuse a clause somebody is still typing. It stays text until it
    // parses, and then becomes a number the engine will accept.
    await user.type(screen.getByLabelText("Value"), "-");
    expect(wire().and[0].value).toBe("-");

    await user.type(screen.getByLabelText("Value"), "12");
    expect(wire().and[0].value).toBe(-12);
  });
});

describe("the join between conditions", () => {
  const MATCH_ALL = "and: match all of these. Press to match any.";

  it("draws no connector until there are two conditions", () => {
    resetIDsForTest();
    render(
      <Harness start={newGroup("and", [newLeaf("owner_id", "eq", "u1")])} />,
    );

    expect(screen.queryByRole("button", { name: MATCH_ALL })).toBeNull();
  });

  it("flips the whole group from either connector", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [
          newLeaf("owner_id", "eq", "u1"),
          newLeaf("full_name", "eq", "ann"),
          newLeaf("cf_deal_score", "gt", 3),
        ])}
      />,
    );

    const connectors = screen.getAllByRole("button", { name: MATCH_ALL });
    expect(connectors.map((button) => button.textContent)).toEqual([
      "and",
      "and",
    ]);
    await user.click(connectors[1]);

    // One group has one join, so every word between its rows changes at once.
    expect(wire()).toEqual({
      or: [
        { field: "owner_id", op: "eq", value: "u1" },
        { field: "full_name", op: "eq", value: "ann" },
        { field: "cf_deal_score", op: "gt", value: 3 },
      ],
    });
    const flipped = screen.getAllByRole("button", {
      name: "or: match any of these. Press to match all.",
    });
    expect(flipped.map((button) => button.textContent)).toEqual(["or", "or"]);
  });
});

describe("groups", () => {
  it("offers a group only from two conditions, and seeds it with one", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness start={newGroup("and", [newLeaf("owner_id", "eq", "u1")])} />,
    );
    expect(
      screen.queryByRole("button", { name: "More for these conditions" }),
    ).toBeNull();

    await user.click(screen.getByRole("button", { name: "Add condition" }));
    await user.click(
      screen.getByRole("button", { name: "More for these conditions" }),
    );
    await user.click(screen.getByRole("button", { name: "Add a group" }));

    // The opposite join, holding a condition: an empty group would stop the
    // count until it was filled.
    expect(wire()).toEqual({
      and: [
        { field: "owner_id", op: "eq", value: "u1" },
        { field: "owner_id", op: "eq", value: "" },
        { or: [{ field: "owner_id", op: "eq", value: "" }] },
      ],
    });
    expect(screen.getByRole("group", { name: "Any of these" })).toBeTruthy();
  });

  it("grows a nested group from its own foot and its own menu", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [
          newLeaf("owner_id", "eq", "u1"),
          newGroup("or", [newLeaf("full_name", "contains", "ann")]),
        ])}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Add condition to group" }),
    );
    expect(wire().and[1]).toEqual({
      or: [
        { field: "full_name", op: "contains", value: "ann" },
        { field: "owner_id", op: "eq", value: "" },
      ],
    });
    // The group's own menu, named apart from the root's.
    expect(
      screen.getByRole("button", { name: "More for this group" }),
    ).toBeTruthy();
  });

  it("says a group the reader emptied matches nothing", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [
          newLeaf("owner_id", "eq", "u1"),
          newGroup("or", [newLeaf("full_name", "contains", "ann")]),
        ])}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Remove Name condition" }),
    );

    expect(screen.getByText("An empty group matches nothing.")).toBeTruthy();
    expect(wire()).toEqual({
      and: [{ field: "owner_id", op: "eq", value: "u1" }, { or: [] }],
    });
  });

  it("removes a nested group without touching its siblings", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [
          newLeaf("owner_id", "eq", "u1"),
          newGroup("or", [newLeaf("full_name", "contains", "ann")]),
        ])}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Remove group" }));

    // The clause beside the group survives: removal names a node, and the root is
    // not offered a remove control at all.
    expect(wire()).toEqual({
      and: [{ field: "owner_id", op: "eq", value: "u1" }],
    });
    expect(screen.queryByRole("button", { name: "Remove group" })).toBeNull();
  });

  it("stops offering a group at the engine's nesting bound", () => {
    resetIDsForTest();
    const two = (a: string, nested: Node) =>
      [newLeaf("owner_id", "eq", a), nested] as const;
    render(
      <Harness
        start={newGroup("and", [
          ...two(
            "u1",
            newGroup("or", [
              ...two(
                "u2",
                newGroup("and", [
                  ...two(
                    "u3",
                    newGroup("or", [
                      ...two("u4", newLeaf("owner_id", "eq", "u5")),
                    ]),
                  ),
                ]),
              ),
            ]),
          ),
        ])}
      />,
    );

    // Every group holds two conditions. Depths 2 and 3 may still nest; depth 4
    // may not, or the tree is one the server refuses as filter_too_deep.
    expect(
      screen.getAllByRole("button", { name: "More for this group" }),
    ).toHaveLength(2);
    expect(
      screen.getByRole("button", { name: "More for these conditions" }),
    ).toBeTruthy();
  });
});

describe("a proposed condition", () => {
  function proposedPair() {
    return newGroup("and", [
      proposedLeaf("full_name", "eq", "Lee"),
      proposedLeaf("cf_deal_score", "gt", 3),
    ]);
  }

  it("is drawn as proposed until the reader changes it", () => {
    resetIDsForTest();
    render(<Harness start={proposedPair()} />);

    expect(document.querySelectorAll("[data-proposed]")).toHaveLength(2);
    expect(screen.getAllByText("Proposed")).toHaveLength(2);
    // The mark is the editor's alone: the wire is the same filter unmarked.
    expect(JSON.stringify(wire())).not.toContain("proposed");
  });

  it("becomes the reader's own on a changed value, and only that row does", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(<Harness start={proposedPair()} />);

    await user.type(screen.getAllByLabelText("Value")[0], "s");

    expect(marks()).toEqual([false, true]);
    expect(document.querySelectorAll("[data-proposed]")).toHaveLength(1);
  });

  it("becomes the reader's own on a changed field or operator", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(<Harness start={proposedPair()} />);

    await pickOption(
      user,
      screen.getAllByRole("combobox", { name: "Operator" })[1],
      "is at least",
    );
    expect(marks()).toEqual([true, false]);

    await pickOption(
      user,
      screen.getAllByRole("combobox", { name: "Field" })[0],
      "Created",
    );
    expect(marks()).toEqual([false, false]);
  });

  it("leaves the other marks alone when one is removed", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(<Harness start={proposedPair()} />);

    await user.click(
      screen.getByRole("button", { name: "Remove Name condition" }),
    );

    expect(marks()).toEqual([true]);
  });
});

describe("the value control follows the operator, then the type", () => {
  it("takes a list for `in` whatever the field's type is", () => {
    resetIDsForTest();
    render(
      <Harness
        start={newGroup("and", [newLeaf("cf_loyalty_tier", "in", [])])}
      />,
    );

    // The token control, not a text box: `in` is a set.
    expect(screen.getByRole("textbox", { name: "Values" })).toBeTruthy();
  });

  it("takes a date box for a date field", () => {
    resetIDsForTest();
    render(
      <Harness
        start={newGroup("and", [newLeaf("created_at", "gte", "2026-07-18")])}
      />,
    );

    const value = screen.getByLabelText("Value");
    expect(value.getAttribute("type")).toBe("date");
  });

  it("asks present-or-empty for `exists` rather than a typed value", async () => {
    resetIDsForTest();
    const user = userEvent.setup();
    render(
      <Harness
        start={newGroup("and", [newLeaf("full_name", "exists", true)])}
      />,
    );

    await user.click(screen.getByRole("button", { name: "is empty" }));

    // `exists: false` is the reading "has no value here" — a real answer, not an
    // unfilled one, which is why the tree stays complete.
    expect(wire()).toEqual({
      and: [{ field: "full_name", op: "exists", value: false }],
    });
  });
});
