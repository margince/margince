// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown, Lock } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { Button } from "./atoms";
import { FactList } from "./factlist";
import { Heading } from "./heading";
import { RecordRefusalContext } from "./lockedvalue";
import "./recorddetails.css";

/**
 * One label/value pair. `empty` folds the row behind "Show N empty fields"
 * until the reader opens them; the caller still passes the plain words for
 * it ("Add…", "Not set"). `block` puts the term over the value, for a value
 * that is a card rather than a line.
 */
export type DetailRow = Readonly<{
  key: string;
  term: ReactNode;
  value: ReactNode;
  empty?: boolean;
  block?: boolean;
}>;

/** A named group: its rows, then anything that is not a row (chips). */
export type DetailSection = Readonly<{
  key: string;
  label: ReactNode;
  rows?: readonly DetailRow[];
  children?: ReactNode;
}>;

/**
 * The details under a record's head, as flat groups: a small label, then
 * label/value rows with no card around them, a hairline between groups. The
 * empty rows of every group fold behind one link, and a group with nothing
 * left to show draws no label. A `refusal` says once, above the fields, why
 * the record is locked, and its fields draw no lock of their own. A page with
 * its own band passes that sentence as `lockedBy`. The `footnote` sits last.
 */
export function RecordDetails({
  sections,
  refusal,
  lockedBy,
  footnote,
}: Readonly<{
  sections: readonly DetailSection[];
  refusal?: Readonly<{ id?: string; text: string; action?: ReactNode }>;
  lockedBy?: string;
  footnote?: ReactNode;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [showEmpty, setShowEmpty] = useState(false);
  const empties = sections.reduce(
    (sum, section) => sum + (section.rows ?? []).filter((r) => r.empty).length,
    0,
  );
  return (
    <div className="recorddetails">
      {refusal && (
        <div className="recorddetails-locked" role="note">
          <Lock size={16} aria-hidden="true" />
          <p id={refusal.id}>{refusal.text}</p>
          {refusal.action}
        </div>
      )}
      <RecordRefusalContext.Provider value={refusal?.text ?? lockedBy}>
        {sections.map((section) => (
          <DetailGroup
            key={section.key}
            section={section}
            showEmpty={showEmpty}
          />
        ))}
      </RecordRefusalContext.Provider>
      {empties > 0 && (
        <div className="recorddetails-foot">
          <Button
            variant="link"
            className="recorddetails-toggle"
            aria-expanded={showEmpty}
            onClick={() => setShowEmpty(!showEmpty)}
          >
            {showEmpty
              ? t("record.details.hideEmpty")
              : plural("record.details.showEmpty", empties, {
                  count: formatNumber(empties, locale),
                })}
            <ChevronDown aria-hidden="true" />
          </Button>
        </div>
      )}
      {footnote && <p className="recorddetails-note">{footnote}</p>}
    </div>
  );
}

function DetailGroup({
  section,
  showEmpty,
}: Readonly<{ section: DetailSection; showEmpty: boolean }>) {
  const labelId = useId();
  const rows = (section.rows ?? []).filter((row) => showEmpty || !row.empty);
  if (rows.length === 0 && !section.children) {
    return null;
  }
  return (
    <section className="recorddetails-group" aria-labelledby={labelId}>
      <Heading
        size="xxsmall"
        as="h2"
        id={labelId}
        className="recorddetails-label"
      >
        {section.label}
      </Heading>
      {rows.length > 0 && <FactList flat facts={rows} />}
      {section.children}
    </section>
  );
}
