// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { useFilterPreview, type VocabularyField } from "../../lib/filterdata";
import { newGroup, newLeaf } from "../../lib/segmentpredicate";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { FilterMatches } from "./filtermatches";

// What a filter selects: the count and the export beside the title, and the
// rows behind it a page at a time. The states are the ones a count can be in:
// on its way, answered and refused. Then a next page on its way, and no match.
const meta: Meta<typeof FilterMatches> = {
  title: "Patterns/Filters and views/Matching records",
  component: FilterMatches,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof FilterMatches>;

const FIELDS: readonly VocabularyField[] = [
  {
    name: "city",
    type: "text",
    operators: ["eq", "neq", "in", "contains", "exists"],
    custom: false,
  },
];

const TREE = newGroup("and", [newLeaf("city", "eq", "Berlin")]);

// Two conditions joined with "and": the empty answer can advise "or".
const AND_TREE = newGroup("and", [
  newLeaf("city", "eq", "Berlin"),
  newLeaf("city", "contains", "Mitte"),
]);

const ROWS = Array.from({ length: 214 }, (_, index) => ({
  id: `p-${index}`,
  full_name: `Contact ${index + 1}`,
  city: "Berlin",
}));

type Answer = Readonly<{ match_count: number; rows: number }>;

/**
 * The preview route: the first page at once, and every later page as the story
 * says. A held page shows one on its way; otherwise the page after the cursor.
 */
function routes(
  first: Answer | "held" | "refused",
  later?: "held",
  extra: RouteMap = {},
): void {
  let asked = 0;
  installFetchStub({
    ...extra,
    "POST /filters/preview": (body) => {
      asked += 1;
      if (first === "held" || (asked > 1 && later === "held")) {
        return new Promise<Response>(() => {});
      }
      if (first === "refused") {
        return jsonResponse(
          {
            title: "Forbidden",
            status: 403,
            code: "seat_tier_insufficient",
            detail: "seat tier insufficient",
          },
          403,
        );
      }
      const read = (key: string) =>
        typeof body === "object" && body !== null && key in body
          ? Reflect.get(body, key)
          : undefined;
      const limit = Number(read("limit") ?? 25);
      const from = Number(read("cursor") ?? 0);
      const to = Math.min(first.rows, from + limit);
      const more = to < first.rows;
      return jsonResponse({
        resource: "contact",
        match_count: first.match_count,
        columns: ["id", "full_name", "city"],
        rows: ROWS.slice(from, to),
        truncated: more,
        next_cursor: more ? String(to) : null,
      });
    },
  });
}

function Matches({ andJoined = false }: Readonly<{ andJoined?: boolean }>) {
  const tree = andJoined ? AND_TREE : TREE;
  const preview = useFilterPreview("contact", tree);
  return (
    <FilterMatches
      preview={preview}
      tab="contacts"
      tree={tree}
      fields={FIELDS}
      named={["city"]}
      andJoined={andJoined}
    />
  );
}

function story(
  first: Answer | "held" | "refused",
  later?: "held",
  andJoined?: boolean,
) {
  return () => {
    routes(first, later);
    return (
      <StoryProviders>
        <Matches andJoined={andJoined} />
      </StoryProviders>
    );
  };
}

// The first count is on its way: no number yet, and no claim that there is none.
export const Counting: Story = { render: story("held") };

export const Counted: Story = {
  render: story({ match_count: 12, rows: 12 }),
};

// The reader stepped past the hundred rows in hand, and the next page is still
// coming. The count stays as it was, and the rows wait for the page.
export const NextPageComing: Story = {
  render: story({ match_count: 214, rows: 214 }, "held"),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    await user.click(
      await canvas.findByRole("combobox", { name: "Rows per page" }),
    );
    await user.click(await page.findByRole("option", { name: "100 per page" }));
    await user.click(await canvas.findByRole("button", { name: "Next" }));
  },
};

// Export stands beside the count. Pressed, it says a file is coming.
export const Exporting: Story = {
  render: () => {
    routes({ match_count: 12, rows: 12 }, undefined, {
      "POST /exports": () => new Promise<Response>(() => {}),
    });
    return (
      <StoryProviders>
        <Matches />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await user.click(await canvas.findByRole("button", { name: "Export CSV" }));
    await canvas.findByText("Exporting…");
  },
};

// A read seat may build a filter and is refused every count.
export const Refused: Story = { render: story("refused") };

// More match than one page holds: the pager walks them a fetch at a time.
export const ManyPages: Story = {
  render: story({ match_count: 214, rows: 214 }),
};

export const NoMatches: Story = {
  render: story({ match_count: 0, rows: 0 }),
};

// Nothing matches two conditions joined with "and", so the advice names the
// connector as well as the most specific condition.
export const NoMatchesAndJoined: Story = {
  render: story({ match_count: 0, rows: 0 }, undefined, true),
};

// Dark: the count beside the title and the table's header band and row rules,
// then the refusal's alert where the count would be.
export const CountedDark: Story = { ...Counted, globals: { theme: "dark" } };

export const RefusedDark: Story = { ...Refused, globals: { theme: "dark" } };
