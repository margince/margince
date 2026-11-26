/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import type { VocabularyField } from "../../lib/filterdata";
import {
  encode,
  type Node,
  newGroup,
  newLeaf,
} from "../../lib/segmentpredicate";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { FilterBuilder } from "./filterbuilder";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ME = meFixture({ allow: { contact: ["read"] } }).user;

const FIELDS: VocabularyField[] = [
  {
    name: "owner_id",
    type: "id",
    operators: ["eq", "neq", "in", "exists"],
    custom: false,
    references: "app_user",
  },
  {
    name: "tag_id",
    type: "id",
    operators: ["eq", "in", "exists"],
    custom: false,
    references: "tag",
  },
  {
    name: "full_name",
    type: "text",
    operators: ["contains", "eq", "exists"],
    custom: false,
  },
];

function serve() {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ allow: { contact: ["read"] } })),
    "GET /users": () =>
      jsonResponse({
        data: [
          { id: "u-z", display_name: "Zoe Park", status: "active" },
          { id: ME.id, display_name: ME.display_name, status: "active" },
          { id: "u-a", display_name: "Ada Brandt", status: "active" },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    "GET /tags": () =>
      jsonResponse({
        data: [{ id: "t-1", name: "VIP" }],
        page: { next_cursor: null, has_more: false },
      }),
  });
}

function Harness({ start }: Readonly<{ start: Node }>) {
  const [tree, setTree] = useState<Node>(start);
  return (
    <StoryProviders>
      <FilterBuilder tree={tree} onChange={setTree} fields={FIELDS} />
      <pre data-testid="wire">{JSON.stringify(encode(tree))}</pre>
    </StoryProviders>
  );
}

const wire = () => JSON.parse(screen.getByTestId("wire").textContent ?? "{}");

describe("a new condition", () => {
  it("names no field, and offers an operator and value once one is picked", async () => {
    serve();
    const user = userEvent.setup();
    render(<Harness start={newGroup("and", [])} />);

    await user.click(
      screen.getByRole("button", { name: en["filters.addClause"] }),
    );

    const field = screen.getByRole("combobox", { name: en["filters.field"] });
    expect(field).toHaveTextContent(en["filters.choosePlaceholder"]);
    expect(screen.queryByRole("combobox", { name: "Operator" })).toBeNull();
    expect(screen.queryByLabelText(en["filters.value"])).toBeNull();
    expect(
      screen.getByRole("button", { name: en["filters.removeEmptyClause"] }),
    ).toBeInTheDocument();

    await pickOption(user, field, "Name");

    // The field's own first operator, not the blank row's placeholder one.
    expect(
      screen.getByRole("combobox", { name: "Operator" }),
    ).toHaveTextContent("contains");
    expect(screen.getByLabelText(en["filters.value"])).toBeInTheDocument();
    expect(wire()).toEqual({
      and: [{ field: "full_name", op: "contains", value: "" }],
    });
  });
});

describe("whether a record has tags", () => {
  it("offers the two readings as what the set includes", async () => {
    serve();
    render(
      <Harness start={newGroup("and", [newLeaf("tag_id", "exists", true)])} />,
    );

    expect(
      await screen.findByRole("button", { name: en["filters.op.includeSome"] }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      screen.getByRole("button", { name: en["filters.op.includeNone"] }),
    ).toHaveAttribute("aria-pressed", "false");
    expect(
      screen.getByRole("combobox", { name: "Operator" }),
    ).toHaveTextContent(en["filters.op.includeSomeOrNone"]);
  });
});

describe("a record picker", () => {
  it("lists you first, then every colleague by name", async () => {
    serve();
    const user = userEvent.setup();
    render(
      <Harness start={newGroup("and", [newLeaf("owner_id", "eq", "")])} />,
    );

    await screen.findByText(en["filters.pickOwner"]);
    await user.click(
      screen.getByRole("combobox", { name: en["filters.value"] }),
    );
    const listbox = await screen.findByRole("listbox");
    const names = within(listbox)
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(names).toEqual([en["filters.you"], "Ada Brandt", "Zoe Park"]);

    await user.click(
      within(listbox).getByRole("option", { name: en["filters.you"] }),
    );
    expect(wire()).toEqual({
      and: [{ field: "owner_id", op: "eq", value: ME.id }],
    });
  });

  it("names the kind of record an empty one asks for", async () => {
    serve();
    render(<Harness start={newGroup("and", [newLeaf("tag_id", "eq", "")])} />);

    await screen.findByText(en["filters.pickTag"]);
    expect(
      screen.getByRole("combobox", { name: en["filters.value"] }),
    ).toHaveTextContent(en["filters.pickTag"]);
  });
});
