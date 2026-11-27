// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { ListPage } from "../../components/listquery";
import {
  listInfiniteOptions,
  listOpening,
} from "../../components/listquery.options";
import {
  type ListQuery,
  type ListReach,
  listFetchLimit,
  listQueryFromParams,
} from "../../components/listqueryparams";
import { CONTACTS_INITIAL_SORT } from "../../lib/listopenings";
import { unwrap } from "../../lib/problem";
import { listQueryParams } from "../../lib/tagfilter";
import { NO_URL_PARAMS } from "../../lib/urlstate";

type Contact = components["schemas"]["Contact"];

export const CONTACTS_LIST_KEY = "contacts";

export async function fetchContactsPage(
  query: ListQuery,
  cursor: string | null,
  signal?: AbortSignal,
  reach?: ListReach,
): Promise<ListPage<Contact>> {
  const data = unwrap(
    await api.GET("/contacts", {
      signal,
      params: {
        query: {
          q: query.q || undefined,
          sort: query.sort || undefined,
          include_archived: query.includeArchived || undefined,
          cursor: cursor || undefined,
          limit: listFetchLimit(query.perPage, reach),
          offset: reach?.offset,
          ...listQueryParams(query.filters),
        },
      },
    }),
  );
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
      total: data.page.total,
    },
  };
}

/**
 * The first read a bare `#/contacts` link opens for the reader `readerScope`
 * names (`healthScope(me)`). The mounted list makes the same request under the
 * same key before the reader turns a dial.
 */
export function contactsArrivalOptions(readerScope: string) {
  const query = listQueryFromParams(
    NO_URL_PARAMS,
    listOpening(CONTACTS_INITIAL_SORT, undefined),
    false,
  );
  return listInfiniteOptions(
    CONTACTS_LIST_KEY,
    readerScope,
    query,
    fetchContactsPage,
  );
}
