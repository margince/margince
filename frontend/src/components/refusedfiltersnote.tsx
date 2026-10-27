// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { currentParams, replaceParams } from "../lib/urlstate";

// The press goes to the address, not the list's query: a list may keep a
// refused dial out of its query, and the address is where the dial still is.
function withoutDials(dials: readonly string[]) {
  const next = new Map(currentParams());
  for (const dial of dials) {
    next.delete(dial);
  }
  replaceParams(next);
}

export function RefusedFiltersNote({
  message,
  dials,
  names,
}: Readonly<{ message: string; dials: readonly string[]; names: string }>) {
  const t = useT();
  return (
    <span>
      {message}{" "}
      <Button onClick={() => withoutDials(dials)}>
        {t("list.filterRefusedRemove", { dial: names })}
      </Button>
    </span>
  );
}
