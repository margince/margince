// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { catalogs, type Locale } from "../i18n";

function escaped(text: string): string {
  return text.replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`);
}

/** A button or panel named for the label it shows; a value follows a comma. */
export function labelled(label: string): RegExp {
  return new RegExp(`^${escaped(label)}(,|$)`);
}

/** The View button, and the panel it opens, in the reader's language. */
export function viewMenuName(locale: Locale = "en"): RegExp {
  return labelled(catalogs[locale]["list.viewMenu"]);
}
