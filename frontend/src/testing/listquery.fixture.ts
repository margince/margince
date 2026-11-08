// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ListQuery } from "../components/listquery";

/** A list with no dial turned: no search, no sort, no filter, live rows only. */
export const UNNARROWED_LIST_QUERY: ListQuery = {
  q: "",
  sort: "",
  includeArchived: false,
  filters: {},
  perPage: 25,
};
