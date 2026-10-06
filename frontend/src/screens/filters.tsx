// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Filters and views destination. An address opens the library, one list,
// or a focused filter page (filtersaddress.ts says which); this file only
// dispatches, so no page imports another.

import { useEffect } from "react";
import { navigateReplacing } from "../app/router";
import { PendingBody } from "../design-system/atoms";
import { useT } from "../i18n";
import { useMe } from "./common";
import { FocusedPending } from "./filterhead";
import { ListFilterPage } from "./filterlistedit";
import { FilterPage } from "./filterpage";
import { fallbackTitleOf, filtersAddressOf } from "./filtersaddress";
import { FiltersLibrary } from "./filterslibrary";
import { OpenedViewPage } from "./filterview";
import { ListPending, ListScreen } from "./listpage";

export function FiltersScreen({
  id,
  list,
  view,
}: Readonly<{ id?: string; list?: string; view?: string }>) {
  const t = useT();
  const me = useMe();
  // `#/lists` names no list, and the lists a reader can find live in the
  // library's Shared group. A redirect, so Back never lands here again.
  useEffect(() => {
    if (list === "") {
      navigateReplacing({ screen: "filters", id: "lists" });
    }
  }, [list]);
  if (list === "") {
    return null;
  }
  const address = filtersAddressOf({ id, id2: view });
  // Nothing draws until the session says whether lists are on: the lists-off
  // library and a page that offers lists would each flash before the page
  // they are not. A page that heads itself still prints its heading while it
  // waits.
  if (me.isPending) {
    if (list !== undefined) {
      return <ListPending />;
    }
    if (address.kind !== "library") {
      return (
        <FocusedPending
          title={t(fallbackTitleOf(address))}
          label={t("common.loading")}
        />
      );
    }
    return (
      <div className="wrap">
        <PendingBody label={t("filters.library.loading")} lines={6} />
      </div>
    );
  }
  // One opened list. It loads with the library that opens it, as one chunk.
  if (list !== undefined) {
    return <ListScreen listID={list} />;
  }
  // Keyed by what each page opened: another record type, view or list is a
  // page of its own rather than one inheriting the last one's draft.
  switch (address.kind) {
    case "library":
      return <FiltersLibrary anchor={address.anchor} />;
    case "new":
      return <FilterPage key={address.tab} tab={address.tab} />;
    case "view":
      return (
        <OpenedViewPage
          key={`${address.tab}/${address.viewId}`}
          tab={address.tab}
          viewId={address.viewId}
        />
      );
    case "listFilter":
      return <ListFilterPage key={address.listId} listId={address.listId} />;
  }
}
