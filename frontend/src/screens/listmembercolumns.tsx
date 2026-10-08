// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The columns a list's member table draws beside each member's name. A Live
// List shows the fields its filter names, so a reader sees at a glance what
// put each record there; a Shortlist shows who chose each record, when, and
// the note they left.

import { Badge } from "../design-system/atoms";
import type { ListColumn } from "../design-system/listtable";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { EntityRef } from "./entityref";
import {
  customColumnLabel,
  fieldLabel,
  useFilterVocabulary,
} from "./filterdata";
import { useFieldValueText, vocabularyField } from "./listexplain";
import { type MemberRow, memberName } from "./listmembers";
import type { List } from "./lists.queries";
import { decode, fieldsNamed } from "./segmentpredicate";

/** How many filter fields are drawn at first; the rest wait in the Display menu. */
export const SHOWN_FILTER_FIELDS = 4;

export function useMemberColumns(
  list: List,
  joined: ReadonlySet<string>,
): ListColumn<MemberRow>[] {
  const t = useT();
  const name: ListColumn<MemberRow> = {
    key: "name",
    header: t("lists.col.name"),
    fixed: true,
    cell: (row) =>
      joined.has(row.id) ? (
        <span className="lists-member-name">
          {memberName(row, t)}
          <Badge tone="discovery">{t("lists.members.new")}</Badge>
        </span>
      ) : (
        memberName(row, t)
      ),
  };
  const filter = useFilterColumns(list);
  const chosen = useChosenColumns();
  return [name, ...(list.list_type === "dynamic" ? filter : chosen)];
}

/**
 * One column per field the filter names, in the order it names them, each
 * cell the member's value as the list read it — "Hidden" where the reader may
 * not see it. Not sortable: the members are read in the record list's order.
 */
function useFilterColumns(list: List): ListColumn<MemberRow>[] {
  const t = useT();
  // Called for a Shortlist too, as hooks are; it has no filter, so asks nothing.
  // A colleague cell names its own id through EntityRef, so no user is asked
  // for up front here.
  const live = list.list_type === "dynamic";
  const vocabulary = useFilterVocabulary(list.entity_type, live);
  const valueText = useFieldValueText(list.entity_type, [], live);
  const tree = decode(list.definition);
  const names = tree ? fieldsNamed(tree) : [];
  return names.map((field, index) => {
    const known = vocabularyField(vocabulary.data, field);
    return {
      key: `field:${field}`,
      header: known ? fieldLabel(known, t) : customColumnLabel(field),
      initiallyHidden: index >= SHOWN_FILTER_FIELDS,
      cell: (row: MemberRow) => {
        const held = row.listing?.values?.[field];
        if (held?.hidden) {
          return <span className="t-caption">{t("lists.members.hidden")}</span>;
        }
        if (held?.value == null) {
          return "—";
        }
        if (held.label == null && known?.references === "app_user") {
          return <EntityRef kind="user" id={held.value} />;
        }
        return held.label ?? valueText(field, held.value);
      },
    };
  });
}

/** Who chose a Shortlist member, on which day, and the note they left. */
function useChosenColumns(): ListColumn<MemberRow>[] {
  const t = useT();
  const { locale } = useLocale();
  return [
    {
      key: "added_by",
      header: t("lists.members.addedBy"),
      cell: (row) => row.listing?.added_by_name ?? "—",
    },
    {
      key: "added_at",
      header: t("lists.members.addedOn"),
      cell: (row) =>
        row.listing?.created_at
          ? formatDate(row.listing.created_at, locale, viewerZone())
          : "—",
    },
    {
      key: "note",
      header: t("lists.members.note"),
      cell: (row) => row.listing?.note ?? "—",
    },
  ];
}
