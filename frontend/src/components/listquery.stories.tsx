// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { ListColumn } from "../design-system/listtable";
import { ProblemError } from "../lib/problem";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import {
  type FilterSpec,
  type ListPage,
  ListTable,
  listFetchLimit,
  useListQuery,
  type ViewSpec,
} from "./listquery";
import type { ListFetchPage } from "./listquery.options";

// ListTable (components/listquery.tsx) binds useListQuery's server-query state
// to the design-system list surface (ListTable in listtable.tsx, imported
// there under the alias ListSurface). It reads the cached ["me"] query, so
// every story needs the same QueryClientProvider + stubbed /me every screen
// story provides, not just LocaleProvider.
function nativeMe() {
  return () =>
    jsonResponse({
      user: { id: "u1", email: "ada@acme.test", display_name: "Ada" },
      roles: ["admin"],
      teams: [],
    });
}

const meta: Meta = {
  title: "Patterns/List query",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      installFetchStub({ "GET /me": nativeMe() });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;

type Story = StoryObj;

type Row = { id: string; name: string; region: string };

const columns: readonly ListColumn<Row>[] = [
  {
    key: "name",
    header: "Name",
    cell: (row) => row.name,
    sort: "full_name",
    fixed: true,
  },
  { key: "region", header: "Region", cell: (row) => row.region },
];

function rows(count: number, offset = 0): Row[] {
  return Array.from({ length: count }, (_, index) => {
    const n = offset + index + 1;
    return {
      id: `r${n}`,
      name: `Contact ${String(n).padStart(2, "0")}`,
      region: n % 2 === 0 ? "EU" : "US",
    };
  });
}

function pageOf(data: Row[], hasMore = false): ListPage<Row> {
  return {
    data,
    page: { next_cursor: hasMore ? "next" : null, has_more: hasMore },
  };
}

function Harness({
  fetchPage,
  chips,
  views,
  readsAtOffset,
}: Readonly<{
  fetchPage: ListFetchPage<Row>;
  chips?: readonly FilterSpec[];
  views?: readonly ViewSpec[];
  readsAtOffset?: boolean;
}>) {
  const state = useListQuery<Row>({
    key: "story-list",
    initialSort: "-created_at",
    fetchPage,
    readsAtOffset,
  });
  return (
    <ListTable
      state={state}
      unit="unit.contacts"
      columns={columns}
      rowKey={(row) => row.id}
      chips={chips}
      views={views}
    />
  );
}

export const Loaded: Story = {
  render: () => <Harness fetchPage={async () => pageOf(rows(3))} />,
};

export const Pending: Story = {
  render: () => (
    <Harness fetchPage={() => new Promise<ListPage<Row>>(() => {})} />
  ),
};

export const ErrorState: Story = {
  render: () => (
    <Harness
      fetchPage={async () => {
        throw new Error("missing scope contacts:read");
      }}
    />
  ),
};

export const Empty: Story = {
  render: () => <Harness fetchPage={async () => pageOf([])} />,
};

// The server refused the list's own sort or filter, as from an old link. A
// retry would ask the same, so the list offers Clear filters instead.
export const RefusedFilters: Story = {
  render: () => (
    <Harness
      fetchPage={async () => {
        throw new ProblemError({ status: 422, detail: "expected one of: a" });
      }}
    />
  ),
};

// Views, a filter chip and a keyset cursor with more than one page: the
// server-side fixture the earlier scenarios don't need, all three
// together.
export const ViewsChipsAndPaging: Story = {
  render: () => (
    <Harness
      fetchPage={async (query, cursor) => {
        const all = rows(60).filter(
          (row) => !query.filters.region || row.region === query.filters.region,
        );
        const from = cursor ? Number(cursor) : 0;
        const size = 25;
        const slice = all.slice(from, from + size);
        const hasMore = from + size < all.length;
        return {
          data: slice,
          page: {
            next_cursor: hasMore ? String(from + size) : null,
            has_more: hasMore,
          },
        };
      }}
      chips={[
        {
          key: "region",
          label: "lead.filterStatus",
          allLabel: "lead.filterStatusAll",
          options: [
            { value: "EU", label: "lead.statusNew" },
            { value: "US", label: "lead.statusContacted" },
          ],
        },
      ]}
      views={[
        { label: "list.viewAll" },
        {
          label: "list.viewMine",
          sort: "full_name",
          filters: { region: "EU" },
        },
      ]}
    />
  ),
};

const COUNTED = 3412;

// A counted list whose endpoint reads at an offset, as /contacts does. A cursor
// names the row it resumes at.
const countedAtOffset: ListFetchPage<Row> = async (
  query,
  cursor,
  _signal,
  reach,
) => {
  const from = reach?.offset ?? Number(cursor ?? 0);
  const to = Math.min(COUNTED, from + listFetchLimit(query.perPage, reach));
  return {
    data: rows(to - from, from),
    page: {
      next_cursor: to < COUNTED ? String(to) : null,
      has_more: to < COUNTED,
      total: cursor === null ? COUNTED : undefined,
    },
  };
};

// The pager numbers the last page the count names, and a press on it reads
// that page at its offset in one request.
export const CountedAndReadAtAnOffset: Story = {
  render: () => <Harness fetchPage={countedAtOffset} readsAtOffset />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Page 137" }),
    );
    await expect(await canvas.findByText("Contact 3401")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Next" })).toBeDisabled();
  },
};

// A list that sends no count: the pages in hand, then a gap while more follow.
export const NotCounted: Story = {
  render: () => (
    <Harness
      fetchPage={async (_query, cursor) => {
        const from = Number(cursor ?? 0);
        return {
          data: rows(50, from),
          page: { next_cursor: String(from + 50), has_more: true },
        };
      }}
    />
  ),
};
