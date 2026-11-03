// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/button";
import { Disclosure } from "../design-system/disclosure";
import { type Fact, FactList } from "../design-system/factlist";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { Receipt } from "../lib/handled";
import { magicByKey } from "../lib/magickeys";
import { useOpenSource } from "../lib/opensource";
import { ChangeList } from "./changelist";

/**
 * One receipt opened: who acted, when, what it was read from, and every value
 * it moved. The server names all of it. A server that predates these fields
 * sends none, and then nothing opens.
 */
export function ReceiptDetail({ receipt }: Readonly<{ receipt: Receipt }>) {
  const t = useT();
  const { locale } = useLocale();
  const openSource = useOpenSource();
  const by = receipt.actor ? magicByKey(receipt.actor.label.key) : null;
  // A source without a label is one the reader may not open. The server leaves
  // it off, and the check holds an older server to the same rule.
  const source = receipt.source_record?.label ? receipt.source_record : null;
  const changes = receipt.changes ?? [];
  if (by === null && source === null && changes.length === 0) {
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
    value: formatDateTime(receipt.occurred_at, locale, viewerZone()),
  });
  if (source !== null) {
    facts.push({
      key: "source",
      term: t("receipt.detail.source"),
      value: openSource ? (
        <Button variant="link" onClick={() => openSource(source.id)}>
          {source.label}
        </Button>
      ) : (
        source.label
      ),
    });
  }
  return (
    <Disclosure summary={t("receipt.detail.summary")}>
      <FactList facts={facts} flat />
      <ChangeList changes={changes} />
    </Disclosure>
  );
}
