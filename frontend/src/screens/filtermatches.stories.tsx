// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { userEvent, within } from "storybook/test";
import { useFilterPreview, type VocabularyField } from "./filterdata";
import { FilterMatches } from "./filtermatches";
import { newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// What a filter selects: the count beside the title, and the first page of
// rows behind it. The states are the ones a count can be in — on its way,
// answered, behind the filter on screen, refused — plus the offer of more rows
// and the answer that nothing matches.
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

const ROWS = Array.from({ length: 100 }, (_, index) => ({
  id: `p-${index}`,
  full_name: `Contact ${index + 1}`,
  city: "Berlin",
}));

type Answer = Readonly<{ match_count: number; rows: number }>;

/**
 * The preview route: the first answer at once, and every later one as the
 * story says — held, to show a count that is behind, or the same again.
 */
function routes(first: Answer | "held" | "refused", later?: "held"): void {
  let asked = 0;
  installFetchStub({
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
      const limit =
        typeof body === "object" && body !== null && "limit" in body
          ? Number(body.limit)
          : 25;
      const rows = ROWS.slice(0, Math.min(first.rows, limit));
      return jsonResponse({
        resource: "contact",
        match_count: first.match_count,
        columns: ["id", "full_name", "city"],
        rows,
        truncated: first.match_count > rows.length,
      });
    },
  });
}

function Matches({ andJoined = false }: Readonly<{ andJoined?: boolean }>) {
  const [limit, setLimit] = useState(25);
  const preview = useFilterPreview(
    "contact",
    andJoined ? AND_TREE : TREE,
    limit,
  );
  return (
    <FilterMatches
      preview={preview}
      tab="contacts"
      fields={FIELDS}
      named={["city"]}
      andJoined={andJoined}
      limit={limit}
      onLimit={setLimit}
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

// The reader asked for a bigger page and the answer is still coming: the last
// count stays on screen, marked as behind.
export const Stale: Story = {
  render: story({ match_count: 214, rows: 100 }, "held"),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    await user.click(
      await canvas.findByRole("combobox", { name: "Rows per page" }),
    );
    await user.click(await page.findByRole("option", { name: "50 per page" }));
  },
};

// A read seat may build a filter and is refused every count.
export const Refused: Story = { render: story("refused") };

// More match than one page holds, so the page offers up to 100.
export const ShowMoreOffered: Story = {
  render: story({ match_count: 214, rows: 100 }),
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
