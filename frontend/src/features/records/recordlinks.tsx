// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The records an activity is filed under, named and reachable.
//
// The link table carries ids and no names, so naming one is a record read.
// Each name is `EntityRef`, so a record the reader may not resolve degrades
// here the same way it does everywhere else in the product.

import { Fragment } from "react";

import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";

type ActivityLink = components["schemas"]["ActivityLink"];

/**
 * Most specific first: a task raised for a lead is ABOUT that lead even when it
 * also names the company the lead came from.
 *
 * Mirrors `linkPriority` in backend/internal/compose/attentionseam.go, which
 * picks the record a Worklist row names. Kept in step so the task panel and the
 * Focus card name the same record first.
 */
const MOST_SPECIFIC_FIRST: readonly ActivityLink["entity_type"][] = [
  "lead",
  "deal",
  "project",
  "contact",
  "company",
];

/**
 * The links in the order above. A stable sort, so two links of one kind keep
 * the order the server sent them in.
 */
export function mostSpecificFirst(
  links: readonly ActivityLink[],
): ActivityLink[] {
  const rank = (link: ActivityLink) =>
    MOST_SPECIFIC_FIRST.indexOf(link.entity_type);
  return [...links].sort((a, b) => rank(a) - rank(b));
}

/**
 * The links as a comma-separated line of record names, or null when there are
 * none. Null rather than an empty fragment: a caller deciding whether to draw a
 * label over the line reads an empty element as truthy.
 */
export function RecordLinks({
  links,
  newTab = false,
}: Readonly<{ links: readonly ActivityLink[]; newTab?: boolean }>) {
  if (links.length === 0) {
    return null;
  }
  return (
    <>
      {links.map((link, index) => (
        <Fragment key={`${link.entity_type}:${link.entity_id}`}>
          {index > 0 && ", "}
          <EntityRef
            kind={link.entity_type}
            id={link.entity_id}
            newTab={newTab}
          />
        </Fragment>
      ))}
    </>
  );
}
