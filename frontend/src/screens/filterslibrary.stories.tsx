// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { FilterVocabulary } from "./filterdata";
import { FiltersScreen } from "./filters";
import {
  listsMe,
  liveList,
  shortlist,
  TEAM_ID,
  teamsPage,
} from "./lists.fixtures";
import type { List } from "./lists.queries";
import type { SavedView } from "./savedviews.queries";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// `#/filters`: every saved view and list the reader can use, in one library.
// The states are the ones a reader can be in: the populated library, a first
// run, lists switched off, a cut, a read that failed or stopped at its cap, a
// search past the cap still out, archived lists shown, a row's ⋯ and the type
// question open, and the wait for the session.
const meta: Meta<typeof FiltersScreen> = {
  title: "Patterns/Filters and views/Library",
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

const view = (
  id: string,
  name: string,
  resource: SavedView["resource"],
  filter: unknown,
): SavedView => ({
  id,
  owner_id: "00000000-0000-4000-8000-000000000001",
  shared_scope: "private",
  resource,
  name,
  query: { filter },
  version: 1,
});

const VIEWS: SavedView[] = [
  view("v1", "Berlin contacts", "contacts", {
    and: [{ field: "city", op: "eq", value: "Berlin" }],
  }),
  view("v2", "Fleet companies", "companies", {
    and: [{ field: "industry", op: "eq", value: "automotive" }],
  }),
  view("v3", "Deals in the last two stages", "deals", {
    and: [{ field: "stage_id", op: "in", value: ["s-4", "s-5"] }],
  }),
];

const LISTS: List[] = [
  {
    ...shortlist,
    id: "L-dinner",
    name: "Dinner guests",
    sharing: "private",
    purpose: "Invited to the October dinner",
    health: "ok",
  },
  {
    ...liveList,
    id: "L-hot",
    name: "Hot leads this quarter",
    entity_type: "lead",
    sharing: "private",
    purpose: "",
    visible_count: 18,
    since_last_visit: undefined,
    definition: { and: [{ field: "classification", op: "eq", value: "hot" }] },
  },
  { ...liveList, id: "L-de", name: "German accounts", team_id: TEAM_ID },
  liveList,
  shortlist,
];

const ARCHIVED: List = {
  ...shortlist,
  id: "L-spring",
  name: "Spring launch invitees",
  sharing: "team",
  health: "ok",
  archived_at: "2026-06-01T00:00:00Z",
};

const VOCABULARY: FilterVocabulary = {
  resource: "contact",
  fields: [
    { name: "city", type: "text", operators: ["eq"], custom: false },
    { name: "industry", type: "text", operators: ["eq"], custom: false },
    {
      name: "stage_id",
      type: "id",
      operators: ["in"],
      custom: false,
      references: "stage",
    },
    {
      name: "classification",
      type: "picklist",
      operators: ["eq"],
      custom: false,
      options: ["hot", "warm", "cold"],
    },
  ],
};

const FAILED = () => jsonResponse({ title: "Server error", status: 500 }, 500);

/** Each story names its own address, since the catalog keeps one window. */
function routes(
  options: Readonly<{
    hash?: string;
    listsOn?: boolean;
    views?: readonly SavedView[];
    lists?: readonly List[];
    listsFail?: boolean;
    truncated?: boolean;
    /** Every `GET /lists` after the first, the capped one, never answers. */
    searchPending?: boolean;
    mePending?: boolean;
  }> = {},
) {
  globalThis.location.hash = options.hash ?? "#/filters";
  let listReads = 0;
  const page = (data: readonly unknown[]) =>
    jsonResponse({ data, page: { has_more: options.truncated === true } });
  installFetchStub({
    "GET /me": options.mePending
      ? () => new Promise<Response>(() => {})
      : listsMe(options.listsOn ?? true, [TEAM_ID]),
    "GET /teams": () => jsonResponse(teamsPage),
    "GET /views": () => page(options.views ?? VIEWS),
    "GET /lists": () => {
      listReads += 1;
      if (options.searchPending && listReads > 1) {
        return new Promise<Response>(() => {});
      }
      return options.listsFail ? FAILED() : page(options.lists ?? LISTS);
    },
    "GET /filters/vocabulary": () => jsonResponse(VOCABULARY),
  });
}

// Both groups, every kind of row: saved views and private lists under Only me,
// team and workspace lists under Shared with who can find each.
export const Populated: Story = {
  render: () => {
    routes();
    return <FiltersScreen />;
  },
};

export const PopulatedDark: Story = {
  ...Populated,
  globals: { theme: "dark" },
};

// Nothing saved yet: one plate, and no search or pills to narrow nothing.
export const FirstRun: Story = {
  render: () => {
    routes({ views: [], lists: [] });
    return <FiltersScreen />;
  },
};

// Lists switched off: one panel of saved views, no kind, no Shortlist verb.
export const ListsOffWithViews: Story = {
  render: () => {
    routes({ listsOn: false });
    return <FiltersScreen />;
  },
};

export const ListsOffEmpty: Story = {
  render: () => {
    routes({ listsOn: false, views: [] });
    return <FiltersScreen />;
  },
};

// The Companies pill pressed: the verb names the record type it will start.
export const CutToCompanies: Story = {
  render: () => {
    routes({ hash: "#/filters?type=companies" });
    return <FiltersScreen />;
  },
};

export const NoHits: Story = {
  render: () => {
    routes({ hash: "#/filters?q=zzz" });
    return <FiltersScreen />;
  },
};

// The lists did not load: Only me keeps its saved views and says what is
// missing; Shared says the same with a retry.
export const PartialFailureListsDown: Story = {
  render: () => {
    routes({ listsFail: true });
    return <FiltersScreen />;
  },
};

// A read stopped at the server's cap: the pills drop their counts, which
// would be floors, and a line says the search reaches further.
export const Truncated: Story = {
  render: () => {
    routes({ truncated: true });
    return <FiltersScreen />;
  },
};

// A search past the cap waits for the server before it says nothing matched.
export const SearchingPastTheCap: Story = {
  render: () => {
    routes({ hash: "#/filters?q=zzz", truncated: true, searchPending: true });
    return <FiltersScreen />;
  },
};

export const ArchivedShown: Story = {
  render: () => {
    routes({ hash: "#/filters?archived=1", lists: [...LISTS, ARCHIVED] });
    return <FiltersScreen />;
  },
};

export const RowMenuOpen: Story = {
  render: () => {
    routes();
    return <FiltersScreen />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "More actions for Berlin contacts",
      }),
    );
  },
};

export const TypePickerOpen: Story = {
  render: () => {
    routes();
    return <FiltersScreen />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New filter" }),
    );
  },
};

export const TypePickerOpenDark: Story = {
  ...TypePickerOpen,
  globals: { theme: "dark" },
};

// At 390px the verb leads at full width, New Shortlist is one press behind
// More, and each row folds its facts under its name.
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => {
    routes();
    return <FiltersScreen />;
  },
};

// The session has not answered: one pending body, and neither layout.
export const MePending: Story = {
  render: () => {
    routes({ mePending: true });
    return <FiltersScreen />;
  },
};
