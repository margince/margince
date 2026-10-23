// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The reference half of a record page's reading: the two sections a reader
// consults rather than reads, side by side.

import type { ReactNode } from "react";
// The pair's own rule, `.co-reading-pair`, lives in company360.css, for the
// reason README.md gives: renaming it reaches four stylesheets. Imported here
// rather than left to the caller, so a page that mounts the pair does not
// render it unstyled.
import "../../companies/company360.css";

/**
 * RecordReadingPair is two reference sections side by side, at half the
 * measure each: a reader consults them rather than reads them through. One
 * column as soon as two would not fit, since a list squeezed to nothing is
 * worse than a fold.
 */
export function RecordReadingPair({
  children,
}: Readonly<{ children: ReactNode }>) {
  return <div className="co-reading-pair">{children}</div>;
}
