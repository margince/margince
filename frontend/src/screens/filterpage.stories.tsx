// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import type { FilterVocabulary } from "./filterdata";
import { useFilterDraft } from "./filterdraft";
import { FilterEditor } from "./filtereditor";
import { FocusedHead } from "./filterhead";
import { usePlainWords } from "./filterpropose";
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
type User = ReturnType<typeof userEvent.setup>;

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

const PROPOSED = {
  and: [
    { field: "city", op: "eq", value: "Hamburg" },
    { field: "last_activity_at", op: "lt", value: { days_ago: 45 } },
  ],
};

function routes(
  options: Readonly<{
    matches?: number;
    refused?: boolean;
    listsOn?: boolean;
  }> = {},
): void {
  const matches = options.matches ?? 3;
  installFetchStub({
    "GET /me": listsMe(options.listsOn === true),
    "POST /filters/propose": () =>
      jsonResponse({ resource: "contact", filter: PROPOSED, unsupported: [] }),
    "GET /filters/vocabulary": () => jsonResponse(VOCABULARY),
    "POST /filters/preview": (body) => {
      if (options.refused) {
        return jsonResponse(READ_SEAT, 403);
      }
      const limit =
        typeof body === "object" && body !== null && "limit" in body
          ? Number(body.limit)
          : 25;
      const rows = ROWS.slice(0, Math.min(matches, limit));
      return jsonResponse({
        resource: "contact",
        match_count: matches,
        columns: ["id", "full_name", "city", "last_activity_at"],
        rows,
        truncated: matches > rows.length,
      });
    },
  });
}

/** One condition on City, written the way a reader writes it. */
async function addCity(user: User, canvas: Canvas, value: string) {
  const adds = await canvas.findAllByRole("button", { name: "Add condition" });
  await user.click(adds[0]);
  const values = canvas.getAllByLabelText("Value");
  const last = values[values.length - 1];
  if (value !== "") {
    await user.type(last, value);
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
    await addCity(userEvent.setup(), within(canvasElement), "");
  },
};

// The first complete condition brings the count, the rows and the footer;
// the description folds away above the row.
export const OneCompleteCondition: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await addCity(user, canvas, "Berlin");
    await canvas.findByText("3 contacts match");
  },
};

// Complete, so the band under the editor says the filter is not kept yet and
// offers the one emerald Save, with the exports in More beside it.
export const CompleteWithFooter: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await addCity(user, canvas, "Berlin");
    await canvas.findByText("Unsaved filter");
  },
};

// Save asks once: a name, and with lists on whether to keep it as a saved
// view or as a Live List.
export const SaveModalOpen: Story = {
  render: newContactFilter({ listsOn: true }),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await addCity(user, canvas, "Berlin");
    await user.click(await canvas.findByRole("button", { name: "Save" }));
  },
};

export const TwoConditionsAnd: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await addCity(user, canvas, "Berlin");
    await addCity(user, canvas, "Hamburg");
  },
};

export const ConnectorFlippedToOr: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await addCity(user, canvas, "Berlin");
    await addCity(user, canvas, "Hamburg");
    await user.click(
      canvas.getByRole("button", {
        name: "and: match all of these. Press to match any.",
      }),
    );
  },
};

/** Two conditions, then More › Add a group. */
async function addGroup(user: User, canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await addCity(user, canvas, "Berlin");
  await addCity(user, canvas, "Hamburg");
  await user.click(
    canvas.getByRole("button", { name: "More for these conditions" }),
  );
  // The menu's items are portalled to the body, outside the story's root.
  await user.click(
    await within(canvasElement.ownerDocument.body).findByRole("button", {
      name: "Add a group",
    }),
  );
}

export const WithAGroup: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    await addGroup(userEvent.setup(), canvasElement);
  },
};

// The reader removed the group's only condition: it says it matches nothing,
// and the count waits for the group to hold one.
export const EmptyGroup: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await addGroup(user, canvasElement);
    const removes = within(canvasElement).getAllByRole("button", {
      name: "Remove City condition",
    });
    await user.click(removes[removes.length - 1]);
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
  const words = usePlainWords({ resource: "contact", dispatch });
  return (
    <div className="wrap filters-screen">
      <FocusedHead title="New contact filter" />
      <Panel title="Find contacts where…">
        <PanelBody>
          <FilterEditor
            draft={draft}
            dispatch={dispatch}
            vocabulary={{ data: VOCABULARY, isPending: false, isError: false }}
            words={words}
            records="contacts"
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

// A proposal landed beside the reader's own condition: the bar above the
// dashed rows, and the footer saying a save keeps them. Save stays offered.
export const ProposalBarWithFooterCount: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await addCity(user, canvas, "Berlin");
    await user.click(
      await canvas.findByText("Describe changes in plain words"),
    );
    await user.type(
      canvas.getByLabelText("Describe the contacts you want"),
      "in Hamburg, quiet for 45 days",
    );
    await user.click(
      canvas.getByRole("button", { name: "Propose conditions" }),
    );
    await canvas.findByText(
      "2 proposed conditions in this filter. Saving keeps them.",
    );
  },
};

// Conditions name one record type's fields, so leaving the type asks first.
export const SwitchTypeAsks: Story = {
  render: newContactFilter(),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await addCity(user, canvas, "Berlin");
    await user.click(canvas.getByRole("button", { name: "Companies" }));
  },
};

// A read seat builds a filter and is refused every count; the count says so
// and the reason and the retry sit where a sentence fits.
export const ReadSeatRefused: Story = {
  render: newContactFilter({ refused: true }),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await addCity(user, canvas, "Berlin");
    await canvas.findByRole("alert");
  },
};

// More match than one page holds: the count line says how many, and the page
// offers up to 100.
export const FirstPageOfMany: Story = {
  render: newContactFilter({ matches: 214 }),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await addCity(user, canvas, "Berlin");
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

// Dark, before and after the first count: the start cards on the page ground,
// then the count, the rows and the footer band, each a mix dark re-derives.
export const CalmStartDark: Story = {
  ...CalmStart,
  globals: { theme: "dark" },
};

export const OneCompleteConditionDark: Story = {
  ...OneCompleteCondition,
  globals: { theme: "dark" },
};
