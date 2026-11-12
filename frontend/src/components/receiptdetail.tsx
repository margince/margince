// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/button";
import { Disclosure } from "../design-system/disclosure";
import { type Fact, FactList } from "../design-system/factlist";
import { formatActivityWhen } from "../format/activitywhen";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { Receipt } from "../lib/handled";
import { magicByKey } from "../lib/magickeys";
import { useOpenSource } from "../lib/opensource";
import { useViewerZone } from "../lib/viewerzone";
import { ChangeList } from "./changelist";

const REASON_KIND: Readonly<Record<string, MessageKey>> = {
  rule: "receipt.detail.reasonKind.rule",
  inference: "receipt.detail.reasonKind.inference",
};

/**
 * One receipt opened: who acted and when, and what it was read from and when
 * that came. Then the kind of reason it rests on, and every value it moved.
 * The server names all of it. A server that predates these fields sends none, and then
 * nothing opens.
 */
export function ReceiptDetail({ receipt }: Readonly<{ receipt: Receipt }>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const openSource = useOpenSource();
  const by = receipt.actor ? magicByKey(receipt.actor.label.key) : null;
  // A source without a label is one the reader may not open. The server leaves
  // it off, and the check holds an older server to the same rule.
  const source = receipt.source_record?.label ? receipt.source_record : null;
  const changes = receipt.changes ?? [];
  const kind = receipt.reason_kind && REASON_KIND[receipt.reason_kind];
  if (by === null && source === null && !kind && changes.length === 0) {
    return null;
  }
  const facts: Fact[] = [];
  if (by !== null) {
    facts.push({
      key: "by",
      term: t("receipt.detail.by"),
      value: t(by, receipt.actor?.label.values),
    });
  }
  facts.push({
    key: "when",
    term: t("receipt.detail.when"),
    value: formatDateTime(receipt.occurred_at, locale, readerZone),
  });
  if (kind) {
    facts.push({
      key: "reasonKind",
      term: t("receipt.detail.reasonKind"),
      value: t(kind),
    });
  }
  if (source !== null) {
    facts.push({
      key: "source",
      term: t("receipt.detail.source"),
      value: openSource ? (
        <Button
          variant="link"
          align="start"
          className="btn-valuelabel"
          onClick={() => openSource(source.id)}
        >
          {source.label}
        </Button>
      ) : (
        source.label
      ),
    });
    if (receipt.source_occurred_at) {
      facts.push({
        key: "sourceAt",
        term: t("receipt.detail.sourceAt"),
        value: formatActivityWhen(
          receipt.source_occurred_at,
          receipt.source_occurred_precision ?? "instant",
          locale,
          readerZone,
        ),
      });
    }
  }
  return (
    <Disclosure summary={t("receipt.detail.summary")}>
      <FactList facts={facts} flat />
      <ChangeList changes={changes} />
    </Disclosure>
  );
}
