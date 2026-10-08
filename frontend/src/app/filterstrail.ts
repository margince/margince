// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Crumb } from "../design-system/breadcrumb";
import { useT } from "../i18n";
import {
  type FiltersAddress,
  filtersAddressOf,
  NEW_FILTER_LABEL,
  opensAFocusedFiltersPage,
} from "../screens/filtersaddress";
import { useList } from "../screens/lists.queries";
import { openableTree, useSavedView } from "../screens/savedviews.queries";
import { type Route, routeHash } from "./router";

/** The list a page names: one list's own page, or its filter's. */
function listIdOf(
  route: Route,
  address: FiltersAddress | null,
): string | undefined {
  if (address?.kind === "listFilter") {
    return address.listId;
  }
  return route.screen === "lists" ? route.id : undefined;
}

/**
 * The trail of a page below the Filters and views library: a focused filter
 * page, or one list. Null on any other route, the library included, whose
 * one-stop trail is the screen's own name. The rail asks the same predicate
 * whether its Filters row is the page or only leads to it (app/nav.ts).
 *
 * A name is the page's own read, watched under the page's key and never asked
 * for here: the trail costs no request, asks nothing of a list while lists are
 * off, and never answers before the page, whose first read of a list is the
 * one that records the visit (listpage.tsx). Until it lands, the stop says
 * what the page is. Both are watched on every route so the hook order never
 * depends on the page.
 */
export function useFiltersCrumbs(route: Route): readonly Crumb[] | null {
  const t = useT();
  const address = route.screen === "filters" ? filtersAddressOf(route) : null;
  const viewId = address?.kind === "view" ? address.viewId : undefined;
  const view = useSavedView(viewId ?? "", false);
  const list = useList(listIdOf(route, address) ?? "", false);

  if (!opensAFocusedFiltersPage(route)) {
    return null;
  }
  const library: Crumb = {
    label: t("filters.title"),
    href: routeHash({ screen: "filters" }),
  };
  const listName = list.data?.name ?? t("lists.page");
  if (address === null) {
    // Outside #/filters the one focused page is a list's own.
    return [library, { label: listName }];
  }
  switch (address.kind) {
    case "library":
      return null;
    case "new":
      return [library, { label: t(NEW_FILTER_LABEL[address.tab]) }];
    case "view": {
      // A view the page cannot open reads as gone there, so it is unnamed here.
      const opened = view.data;
      const viewName =
        opened && openableTree(opened) !== null
          ? opened.name
          : t("filters.library.kindView");
      return [library, { label: viewName }];
    }
    case "listFilter":
      return [
        library,
        {
          label: listName,
          href: routeHash({ screen: "lists", id: address.listId }),
        },
        { label: t("lists.editFilter") },
      ];
  }
}
