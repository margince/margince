// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import type { FilterVocabulary } from "./filterdata";
import { useFilterDraft } from "./filterdraft";
import { FilterEditor } from "./filtereditor";
import { FocusedHead } from "./filterhead";
import { FiltersScreen } from "./filters";
import { listsMe } from "./lists.fixtures";
import { newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// `#/filters/contacts`: a new filter, from the calm start to a filter worth
// keeping. The page grows only as the reader builds, so most states are
// reached the way a reader reaches them — by pressing through the page.
const meta: Meta<typeof FiltersScreen> = {
  title: "Patterns/Filters and views/New filter",
  component: FiltersScreen,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof FiltersScreen>;
type Canvas = ReturnType<typeof within>;

const VOCABULARY: FilterVocabulary = {
  resource: "contact",
  fields: [
    {
      name: "city",
      type: "text",
      operators: ["eq", "neq", "in", "contains", "exists"],
      custom: false,
    },
    {
      name: "last_activity_at",
      type: "date",
      operators: ["eq", "neq", "gt", "gte", "lt", "lte", "exists"],
      custom: false,
    },
  ],
};

const ROWS = Array.from({ length: 100 }, (_, index) => ({
  id: `p-${index}`,
  full_name: `Contact ${index + 1}`,
  city: "Berlin",
  last_activity_at: "2026-08-01",
}));

const READ_SEAT = {
  title: "Forbidden",
  status: 403,
  code: "seat_tier_insufficient",
  detail: "seat tier insufficient",
};

function routes(
  options: Readonly<{ matches?: number; refused?: boolean }> = {},
): void {
  const matches = options.matches ?? 3;
  installFetchStub({
    "GET /me": listsMe(false),
    "GET /filters/vocabulary": () => jsonResponse(VOCABULARY),
    "POST /filters/preview": (body) => {
      if (options.refused) {
        return jsonResponse(READ_SEAT, 403);
      }
      const limit =
        typeof body === "object" && body !== null && "limit" in body
          ? Number(body.limit)
          : 25;
      return jsonResponse({
        resource: "contact",
        match_count: matches,
        columns: ["id", "full_name", "city", "last_activity_at"],
        rows: ROWS.slice(0, Math.min(matches, limit)),
        truncated: false,
      });
    },
  });
}

/** One condition on City, written the way a reader writes it. */
async function addCity(canvas: Canvas, value: string) {
  const adds = await canvas.findAllByRole("button", { name: "Add condition" });
  await userEvent.click(adds[0]);
  const values = canvas.getAllByLabelText("Value");
  const last = values[values.length - 1];
  if (value !== "") {
    await userEvent.type(last, value);
  }
}

function newContactFilter(options?: Parameters<typeof routes>[0]) {
  return () => {
    routes(options);
    return <FiltersScreen id="contacts" />;
  };
}

// The page as it opens: its name, the record type, two ways in, and the line
// saying what would bring a count. One emerald control.
export const CalmStart: Story = { render: newContactFilter() };

export const OneIncompleteCondition: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    await addCity(within(canvasElement), "");
  },
};

// The first complete condition brings the count, the rows and the footer;
// the description folds away above the row.
export const OneCompleteCondition: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await addCity(canvas, "Berlin");
    await canvas.findByText("3 contacts match");
  },
};

export const TwoConditionsAnd: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await addCity(canvas, "Berlin");
    await addCity(canvas, "Hamburg");
  },
};

export const ConnectorFlippedToOr: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await addCity(canvas, "Berlin");
    await addCity(canvas, "Hamburg");
    await userEvent.click(
      canvas.getByRole("button", {
        name: "and: match all of these. Press to match any.",
      }),
    );
  },
};

/** Two conditions, then More › Add a group. */
async function addGroup(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await addCity(canvas, "Berlin");
  await addCity(canvas, "Hamburg");
  await userEvent.click(
    canvas.getByRole("button", { name: "More for these conditions" }),
  );
  // The menu's items are portalled to the body, outside the story's root.
  await userEvent.click(
    await within(canvasElement.ownerDocument.body).findByRole("button", {
      name: "Add a group",
    }),
  );
}

export const WithAGroup: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    await addGroup(canvasElement);
  },
};

// The reader removed the group's only condition: it says it matches nothing,
// and the count waits for the group to hold one.
export const EmptyGroup: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    await addGroup(canvasElement);
    const removes = within(canvasElement).getAllByRole("button", {
      name: "Remove City condition",
    });
    await userEvent.click(removes[removes.length - 1]);
  },
};

/** The page's editor holding conditions a model proposed, marked by hand. */
function ProposedRows() {
  const [draft, dispatch] = useFilterDraft(() =>
    newGroup("and", [
      newLeaf("city", "eq", "Berlin"),
      { ...newLeaf("city", "eq", "Hamburg"), proposed: true },
      {
        ...newLeaf("last_activity_at", "lt", { days_ago: 45 }),
        proposed: true,
      },
    ]),
  );
  return (
    <div className="wrap filters-screen">
      <FocusedHead title="New contact filter" />
      <Panel title="Find contacts where…">
        <PanelBody>
          <FilterEditor
            resource="contact"
            draft={draft}
            dispatch={dispatch}
            vocabulary={{ data: VOCABULARY, isPending: false, isError: false }}
          />
        </PanelBody>
      </Panel>
    </div>
  );
}

// Proposed rows among the reader's own: the staged edge and the Proposed
// badge, on rows the reader can still change.
export const ProposedRowsHandMarked: Story = {
  render: () => {
    routes();
    return <ProposedRows />;
  },
};

// Conditions name one record type's fields, so leaving the type asks first.
export const SwitchTypeAsks: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await addCity(canvas, "Berlin");
    await userEvent.click(canvas.getByRole("button", { name: "Companies" }));
  },
};

// A read seat builds a filter and is refused every count; the count says so
// and the reason and the retry sit where a sentence fits.
export const ReadSeatRefused: Story = {
  render: newContactFilter({ refused: true }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await addCity(canvas, "Berlin");
    await canvas.findByRole("alert");
  },
};

// More match than one page holds: the count line says how many, and the page
// offers up to 100.
export const FirstPageOfMany: Story = {
  render: newContactFilter({ matches: 214 }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await addCity(canvas, "Berlin");
    await canvas.findByText("214 contacts match");
  },
};

// At 390px the record type drops under the name at full width, and the two
// ways in stack.
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: newContactFilter(),
};
