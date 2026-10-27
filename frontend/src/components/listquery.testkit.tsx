import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import type { ListChip } from "../design-system/listsurface";
import { LocaleProvider } from "../i18n";
import {
  type FilterSpec,
  type ListPage,
  type ListQuery,
  type ListState,
  ListTable,
  type SavedViewTab,
  useListQuery,
  type ViewSpec,
} from "./listquery";

export function render(ui: ReactNode): RenderResult {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

export type Row = { id: string; name: string };

export function emptyPage(): ListPage<Row> {
  return { data: [], page: { next_cursor: null, has_more: false } };
}

export function ListTableHarness({
  fetchPage,
  chips,
  action,
  views,
  dataViews,
  dataChips,
  scopeKey,
  initialFilters,
  title,
  emptyNote,
  hideShortChrome,
}: Readonly<{
  fetchPage: (
    query: ListQuery,
    cursor: string | null,
  ) => Promise<ListPage<Row>>;
  chips?: readonly FilterSpec[];
  action?: ReactNode;
  views?: readonly ViewSpec[];
  dataViews?: readonly SavedViewTab[];
  dataChips?: readonly ListChip[];
  scopeKey?: string;
  initialFilters?: Readonly<Record<string, string>>;
  title?: string;
  emptyNote?: (state: ListState<Row>) => ReactNode;
  hideShortChrome?: boolean;
}>) {
  const state = useListQuery<Row>({
    key: "list-table-harness",
    initialSort: "-created_at",
    initialFilters,
    fetchPage,
  });
  return (
    <ListTable
      title={title}
      emptyNote={emptyNote?.(state)}
      hideShortChrome={hideShortChrome}
      state={state}
      unit="nav.contacts"
      columns={[
        {
          key: "name",
          header: "contacts.name",
          cell: (row: Row) => row.name,
          sort: "full_name",
        },
      ]}
      rowKey={(row) => row.id}
      chips={chips}
      dataChips={dataChips}
      views={views}
      dataViews={dataViews}
      action={action}
      scopeKey={scopeKey}
    />
  );
}
