// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";

// What the document-sets card says about ITSELF: a set being re-read, a file it
// could not read, and the files a drop left behind. None of them is content —
// every one is a claim about the surface. A write the server refused is the
// shared notice in `common.tsx`, because every card in the product has one.

/** A set being re-read has nothing for the reader to do but wait, and saying so
 *  is different from saying it is not ready. */
export function ReindexingNotice() {
  const t = useT();
  return (
    <Callout kind="event" title={t("knowledge.reindexingTitle")}>
      {t("knowledge.reindexing")}
    </Callout>
  );
}

/** The reason a failed ingest carries. A set quietly short of a file nobody can
 *  name answers worse than an empty one, because it still answers. */
export function IngestFailure({ detail }: Readonly<{ detail: string }>) {
  const t = useT();
  return (
    <Callout
      kind="event"
      tone="danger"
      title={t("knowledge.ingestDetailTitle")}
    >
      {detail}
    </Callout>
  );
}

/**
 * One refused file out of several. Named, because "3 of 10 failed" leaves the
 * reader to work out WHICH three by comparing two lists by eye.
 *
 * `id` is the file's OWN identity, not its position in the batch: two refused
 * files may share a name now that both are kept, and a key that changes when
 * the list is rebuilt would let React reuse the wrong message.
 */
export type UploadRefusal = Readonly<{
  id: string;
  filename: string;
  message: string;
}>;

/**
 * ONE verdict for the drop, naming each file it refused.
 *
 * N stacked callouts made the reader read the same heading once per file to
 * find the three names that differed.
 */
export function UploadRefusals({
  refusals,
}: Readonly<{ refusals: readonly UploadRefusal[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (refusals.length === 0) return null;
  return (
    <Callout
      kind="outcome"
      tone="danger"
      title={plural("knowledge.upload.refusedTitle", refusals.length, {
        count: formatNumber(refusals.length, locale),
      })}
    >
      <ul>
        {refusals.map((refusal) => (
          <li key={refusal.id}>
            {t("knowledge.upload.refused", {
              filename: refusal.filename,
              message: refusal.message,
            })}
          </li>
        ))}
      </ul>
    </Callout>
  );
}
