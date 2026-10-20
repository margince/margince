// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { formatDateAbbrev } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useMemberName } from "../lib/membernames";
import { useRecordZone } from "../lib/recordzone";
import { useViewerId } from "../lib/session";
import { Eyebrow } from "./eyebrow";
import { Popover } from "./popover";
import { ProvenanceTag, type SourceAuthor } from "./provenance";
import { provenanceOf } from "./provenanceof";

// The head's facts strip: caption over value, cells laid along the row and
// wrapping when the head narrows. Every record's head has one (the contact
// page wrote it first, as `.pe-facts`), and the company and lead pages are
// growing the same strip under their own names, so it moves here rather than
// staying a contact-only spelling a second page would have to copy.

/**
 * The strip itself: a `dl`, because each cell IS a term and its value.
 * Children are the `Fact` cells; a page's own facts (which to show, in what
 * order, whether one is even known) are the caller's, not this component's.
 */
export function RecordFacts({ children }: Readonly<{ children: ReactNode }>) {
  return <dl className="record-facts">{children}</dl>;
}

/**
 * One cell: a label over a value. The label is `Eyebrow`, so the strip reads
 * with the same micro-type every other record head uses for a caption.
 */
export function Fact({
  label,
  named,
  children,
}: Readonly<{
  label: string;
  // Prints the label beside the value in a record head's one-line form, for a
  // value that does not say what it is on its own.
  named?: boolean;
  children: ReactNode;
}>) {
  return (
    <div className={named ? "record-fact record-fact-named" : "record-fact"}>
      <dt>
        <Eyebrow>{label}</Eyebrow>
      </dt>
      <dd>{children}</dd>
    </div>
  );
}

/** The columns of a record row that say where it came from. */
export type RecordOrigin = Readonly<{
  created_at?: string;
  captured_by?: string;
  author?: SourceAuthor | null;
  source?: string | null;
  imported_from?: string | null;
}>;

/**
 * RecordOriginFacts is when a record's row was written and by whom. It is a
 * fact about the record, not about what the record names. So it stands in the
 * fields column and not in the line under the name.
 */
export function RecordOriginFacts({
  record,
}: Readonly<{ record: RecordOrigin }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const viewerId = useViewerId();
  const provenance = provenanceOf(
    record.captured_by,
    viewerId,
    record.author,
    record.source,
    record.imported_from,
  );
  const authorName = useMemberName(
    provenance.kind === "human" ? provenance.userId : undefined,
  );
  return (
    <RecordFacts>
      {record.created_at ? (
        <Fact label={t("list.created")} named>
          {formatDateAbbrev(record.created_at, locale, zone)}
        </Fact>
      ) : null}
      {/* The tag names the KIND of source; the popover behind it answers
          WHICH one, which for a mailbox capture is a message id no reader
          should have to scan past. */}
      <Fact label={t("history.field.source")}>
        <Popover
          label={
            <ProvenanceTag
              provenance={provenance}
              renderUser={() => authorName.data ?? undefined}
            />
          }
        >
          <p className="t-body">{record.source || t("trust.sourceUnknown")}</p>
        </Popover>
      </Fact>
    </RecordFacts>
  );
}
