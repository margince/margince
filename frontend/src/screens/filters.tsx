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
import { FilterPage } from "./filterpage";
import { fallbackTitleOf, filtersAddressOf } from "./filtersaddress";
import { FiltersLibrary } from "./filterslibrary";
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
  if (address.kind === "library") {
    return <FiltersLibrary anchor={address.anchor} />;
  }
  return <FilterPage address={address} />;
}
