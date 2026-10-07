// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ListScreen } from "./listpage";
import {
  changesSinceVisit,
  chosenListing,
  exportDependencies,
  history,
  LIVE_ID,
  listingAnswer,
  listsMe,
  liveHistory,
  liveList,
  liveListing,
  liveVocabulary,
  members,
  OTHER_OWNER_ID,
  SHORTLIST_ID,
  shortlist,
  visitAnswer,
} from "./lists.fixtures";
import type { List } from "./lists.queries";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// One list opened: its name and verbs, what it is for, the facts about it, its
// members and what changed. The list page heads itself, so the story is
// fullscreen.
const meta: Meta = {
  title: "Records/List page",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

const page = { has_more: false };

// The page opened on `list`, answering every read it makes; `routes` replaces
// any of those answers.
function opened(list: List, routes: RouteMap = {}) {
  const live = list.list_type === "dynamic";
  return () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${list.id}`]: () => jsonResponse(list),
      [`POST /lists/${list.id}/visit`]: visitAnswer(list.id),
      [`GET /lists/${list.id}/history`]: () =>
        jsonResponse({ data: live ? liveHistory : history, page }),
      "GET /companies": () => jsonResponse({ data: members, page }),
      [`GET /lists/${list.id}/members`]: listingAnswer(
        live ? liveListing : chosenListing,
      ),
      "GET /filters/vocabulary": () => jsonResponse(liveVocabulary),
      ...routes,
    });
    return (
      <StoryProviders>
        <ListScreen listID={list.id} />
      </StoryProviders>
    );
  };
}

// A Live List a reader can change: Export CSV, Edit filter and the menu beside
// its name, its filter read as one sentence under its purpose, and members from
// that filter, each beside the values of the fields the filter names.
export const LiveList: Story = { render: opened(liveList) };

// A Shortlist whose steward is gone: the notice asks somebody to take it over,
// the facts name no steward, and each member says who chose it, when and why.
export const ShortlistNobodyLooksAfter: Story = {
  render: opened(shortlist),
};

// An archived list is read-only: no menu and no Edit filter, only the notice's
// Restore. It keeps its members and history, so Export CSV stays.
export const Archived: Story = {
  render: opened({ ...liveList, archived_at: "2026-09-25T10:00:00Z" }),
};

// Archived with its steward gone: the archived notice wins, so the head's badge
// says the list needs a steward and the facts name nobody.
export const ArchivedNobodyLooksAfter: Story = {
  render: opened({
    ...liveList,
    archived_at: "2026-09-25T10:00:00Z",
    health: "ownerless",
    steward_id: null,
    steward_name: null,
  }),
};

// With lists switched off the page says so, and reads nothing.
export const ListsSwitchedOff: Story = {
  render: () => {
    installFetchStub({ "GET /me": listsMe(false) });
    return (
      <StoryProviders>
        <ListScreen listID={LIVE_ID} />
      </StoryProviders>
    );
  },
};

// A new Live List the checks have not reached: What changed says so above its
// history, which holds only the list's own changes until then.
export const NotCheckedYet: Story = {
  render: opened(
    {
      ...liveList,
      last_check: undefined,
      since_last_visit: undefined,
      joined_since_visit: [],
    },
    {
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({
          data: [
            {
              id: "01a0f000-0000-7000-8000-000000000013",
              kind: "revised",
              occurred_at: "2026-09-30T08:40:00Z",
              actor: "human:00000000-0000-4000-8000-000000000001",
              actor_name: "Lena Vogt",
            },
          ],
          page,
        }),
    },
  ),
};

// Since the reader's last visit, records joined and left and the filter
// changed: one sentence under the filter, whose records open.
export const ChangedSinceVisit: Story = {
  render: opened({ ...liveList, changes_since_visit: changesSinceVisit }),
};

// Exported twice: the Exported fact counts both and dates the newer.
export const Exported: Story = {
  render: opened({ ...liveList, dependencies: exportDependencies }),
};

// A filter that no longer compiles: the list cannot count or read its members,
// so Records names the type alone, Members says it cannot show them, and the
// one notice offers Edit filter.
export const FilterNoLongerWorks: Story = {
  render: opened(
    {
      ...liveList,
      health: "invalid",
      visible_count: null,
      since_last_visit: undefined,
      joined_since_visit: [],
    },
    {
      "GET /companies": () =>
        jsonResponse(
          {
            title: "Unprocessable filter",
            status: 422,
            detail: "The list's filter no longer compiles.",
          },
          422,
        ),
    },
  ),
};

// The filter names a field that was retired: the notice names it and offers
// Edit filter, beside the head's own.
export const RetiredField: Story = {
  render: opened({
    ...liveList,
    health: "retired_field",
    retired_fields: ["cf_last_touch"],
  }),
};

// A reader who may not change the list keeps Export CSV and nothing else.
export const ReadOnly: Story = {
  render: opened({
    ...liveList,
    can_edit: false,
    owner_id: OTHER_OWNER_ID,
    steward_id: OTHER_OWNER_ID,
    steward_name: "Jonas Brandt",
  }),
};

// A Shortlist of projects: the record lists do not serve projects, so Members
// says where to find them and there is no Export CSV.
export const ProjectList: Story = {
  render: opened(
    {
      ...shortlist,
      name: "Autumn fit-outs",
      purpose: "Projects we staff from the Hamburg office",
      entity_type: "project",
      steward_id: OTHER_OWNER_ID,
      steward_name: "Jonas Brandt",
      visible_count: 5,
      health: "ok",
    },
    {
      [`GET /lists/${SHORTLIST_ID}/history`]: () =>
        jsonResponse({ data: [], page }),
    },
  ),
};

// A list that is gone, or that this reader may not see: one sentence and the
// way back to the library.
export const Gone: Story = {
  render: opened(liveList, {
    [`GET /lists/${LIVE_ID}`]: () =>
      jsonResponse(
        { title: "Not found", status: 404, detail: "list not found" },
        404,
      ),
  }),
};

// The list is still being read: the page's own name and a skeleton.
export const Pending: Story = {
  render: opened(liveList, {
    [`GET /lists/${LIVE_ID}`]: () => new Promise<Response>(() => {}),
  }),
};

// Dark, on the states that draw a notice, the stripped members table on the
// Panel ground, and the head's verbs.
export const LiveListDark: Story = {
  ...LiveList,
  globals: { theme: "dark" },
};

export const ShortlistNobodyLooksAfterDark: Story = {
  ...ShortlistNobodyLooksAfter,
  globals: { theme: "dark" },
};

export const ArchivedDark: Story = {
  ...Archived,
  globals: { theme: "dark" },
};

export const FilterNoLongerWorksDark: Story = {
  ...FilterNoLongerWorks,
  globals: { theme: "dark" },
};

export const RetiredFieldDark: Story = {
  ...RetiredField,
  globals: { theme: "dark" },
};

export const ReadOnlyDark: Story = {
  ...ReadOnly,
  globals: { theme: "dark" },
};

// At phone width the verbs drop under the name, the facts wrap, and the
// members fold into the table's own phone rows. `uat-phone` is what drives the
// capture gate's browser to 390px; the viewport global alone never reaches it.
export const LiveListPhone: Story = {
  ...LiveList,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const ShortlistNobodyLooksAfterPhone: Story = {
  ...ShortlistNobodyLooksAfter,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
