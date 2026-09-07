// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { SpokenLine } from "./ai-activity-lines";
import { routeHash } from "./router";

/**
 * One of the rail's lines, with the record it names as the way to that record.
 *
 * The name is a link only where the line's subject has a page; a document or a
 * meeting is drawn as the word it is. A BARE anchor on purpose: the base
 * stylesheet draws the classless link in a sentence, and this is exactly that —
 * a name inside the agent's own words, not a control of the rail's.
 *
 * A plain `href` rather than a click handler: the address is the product's
 * own, so a middle click, a copied link and a screen reader's link list all get
 * the record too.
 */
export function RailLine({ line }: Readonly<{ line: SpokenLine }>) {
  if (line.subject === null) {
    return <>{line.before}</>;
  }
  const { name, route } = line.subject;
  return (
    <>
      {line.before}
      {route === null ? name : <a href={routeHash(route)}>{name}</a>}
      {line.after}
    </>
  );
}
