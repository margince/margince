// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The records an activity is filed under, named and reachable.
//
// The link table carries ids and no names, so naming one is a record read.
// Each name is `EntityRef`, so a record the reader may not resolve degrades
// here the same way it does everywhere else in the product.

import {
  FolderKanban,
  Handshake,
  type LucideIcon,
  UserPlus,
} from "lucide-react";
import { Fragment } from "react";

import type { components } from "../api/schema";
import { Avatar } from "../design-system/avatar";
import { Chip } from "../design-system/readings";
import { recordEnabled, useDisabledCoreFeatures } from "../lib/corefeatures";
import { ENTITY } from "../lib/entity";
import { valueFor } from "../lib/labelfor";
import { routeHash } from "../lib/router";
import { EntityRef } from "./entityref";
import { useEntityName } from "./entityref.queries";

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
  const disabled = useDisabledCoreFeatures();
  const available = links.filter((link) =>
    recordEnabled(link.entity_type, disabled),
  );
  if (available.length === 0) {
    return null;
  }
  return (
    <>
      {available.map((link, index) => (
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

/**
 * The mark a record chip carries for a kind with no face. A contact or a
 * company is drawn as its Avatar instead, so it has no entry here.
 */
export const RECORD_CHIP_ICONS: Partial<
  Record<ActivityLink["entity_type"], LucideIcon>
> = {
  deal: Handshake,
  lead: UserPlus,
  project: FolderKanban,
};

/**
 * The links as a row of chips, the way a Home card names who and what a task
 * is with. Each chip opens its record in place.
 */
export function RecordLinkChips({
  links,
}: Readonly<{ links: readonly ActivityLink[] }>) {
  const disabled = useDisabledCoreFeatures();
  return links
    .filter((link) => recordEnabled(link.entity_type, disabled))
    .map((link) => (
      <RecordLinkChip
        key={`${link.entity_type}:${link.entity_id}`}
        link={link}
      />
    ));
}

function RecordLinkChip({ link }: Readonly<{ link: ActivityLink }>) {
  const { entity_type: kind, entity_id: id } = link;
  const { name } = useEntityName(kind, id);
  const icon = RECORD_CHIP_ICONS[kind];
  const mark = icon
    ? { icon }
    : { lead: <Avatar name={name ?? id} identity={id} /> };
  const entity = valueFor(ENTITY, kind, null);
  // Only a resolved name is a link; until then the chip says what the read
  // knows, the way every other reference to a record does.
  return name && entity ? (
    <Chip dense appHref={routeHash(entity.route(id))} {...mark}>
      {name}
    </Chip>
  ) : (
    <Chip dense {...mark}>
      <EntityRef kind={kind} id={id} asText />
    </Chip>
  );
}
