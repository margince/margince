// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What changed on a Live List since the reader's last visit, as one sentence
// in which every named record opens. The server counts it from the list's
// history under the reader's row scope; nothing here is inferred.

import { Fragment } from "react";
import { Button } from "../design-system/atoms";
import { formatDateAbbrev, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { List } from "./lists.queries";

type Summary = NonNullable<List["changes_since_visit"]>;
type Group = Summary["joined"];

export function ListChangeSummary({
  summary,
  onOpen,
}: Readonly<{
  summary: Summary;
  /** Opens a named record; absent for a record type with no page. */
  onOpen?: (id: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const count = (n: number) => formatNumber(n, locale);
  const moved = (["joined", "left"] as const).filter(
    (direction) => summary[direction].count > 0,
  );
  return (
    <p className="t-caption">
      {t("lists.changes.since", {
        when: formatDateAbbrev(summary.since, locale, viewerZone()),
      })}{" "}
      {moved.length === 0
        ? t("lists.changes.nothing")
        : moved.map((direction, i) => (
            <Fragment key={direction}>
              {i > 0 && ", "}
              {plural(`lists.changes.${direction}`, summary[direction].count, {
                count: count(summary[direction].count),
              })}
              <Names group={summary[direction]} onOpen={onOpen} />
            </Fragment>
          ))}
      {moved.length > 0 && "."}
      {summary.filter_changes > 0 && (
        <>
          {" "}
          {plural("lists.changes.filter", summary.filter_changes, {
            count: count(summary.filter_changes),
          })}
        </>
      )}
    </p>
  );
}

/** The newest records of one direction by name, and how many more moved. */
function Names({
  group,
  onOpen,
}: Readonly<{ group: Group; onOpen?: (id: string) => void }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const more = group.count - group.records.length;
  if (group.records.length === 0) {
    return null;
  }
  return (
    <>
      {" ("}
      {group.records.map((record, i) => {
        const name = record.name ?? t("lists.unnamed");
        return (
          <Fragment key={record.entity_id}>
            {i > 0 && ", "}
            {onOpen ? (
              <Button variant="link" onClick={() => onOpen(record.entity_id)}>
                {name}
              </Button>
            ) : (
              name
            )}
          </Fragment>
        );
      })}
      {more > 0 &&
        `, ${plural("lists.changes.more", more, { count: formatNumber(more, locale) })}`}
      {")"}
    </>
  );
}
