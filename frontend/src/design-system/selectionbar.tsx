// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
// The bar's rule sits with the table that drew it first.
import "./listtable.css";

/**
 * The bar a selection's count and verbs stand in. `ListTable` draws it over
 * its grid; a list that is not a table draws it over its rows.
 */
export function SelectionBar({ children }: Readonly<{ children: ReactNode }>) {
  return (
    /* aria-live and no role="region": the announcement is what this element is
       for, and aria-live delivers it on any element. The landmark did not — a
       region must be named to be worth anything, this one never was, and an
       anonymous landmark in the list costs a reader a stop that tells them
       nothing. */
    <div className="lt-bulkbar" aria-live="polite">
      {children}
    </div>
  );
}
