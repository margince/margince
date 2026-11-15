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
  listFetchLimit,
  listQueryFromParams,
} from "../../components/listqueryparams";
import { unwrap } from "../../lib/problem";
import { listQueryParams } from "../../lib/tagfilter";
import { NO_URL_PARAMS } from "../../lib/urlstate";

type Contact = components["schemas"]["Contact"];

export const CONTACTS_LIST_KEY = "contacts";

/** The order the Contacts list opens on before a reader picks one. */
export const CONTACTS_INITIAL_SORT = "-created_at";

export async function fetchContactsPage(
  query: ListQuery,
  cursor: string | null,
  signal?: AbortSignal,
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
          limit: listFetchLimit(query.perPage),
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
 * The first read a bare `#/contacts` link opens. The mounted list makes the
 * same request under the same key before the reader turns a dial.
 */
export function contactsArrivalOptions() {
  const query = listQueryFromParams(
    NO_URL_PARAMS,
    listOpening(CONTACTS_INITIAL_SORT, undefined),
    false,
  );
  return listInfiniteOptions(CONTACTS_LIST_KEY, query, fetchContactsPage);
}
