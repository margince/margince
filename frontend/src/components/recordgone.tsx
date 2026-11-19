// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A record address that opens no record. A missing record and one outside the
// reader's row scope both answer 404, so the sentence names both. The way on is
// the record's list, since no retry can make the record appear.

import { Button, EmptyState } from "../design-system/atoms";
import { isMessageKey, useT } from "../i18n";
import { ENTITY, type EntityKind } from "../lib/entity";
import { navigate } from "../lib/router";

export function RecordGone({ kind }: Readonly<{ kind: EntityKind | "offer" }>) {
  const t = useT();
  // An offer has no list of its own; it is reached from its deal.
  const list = ENTITY[kind === "offer" ? "deal" : kind].route("").screen;
  const name = `nav.${list}`;
  return (
    <EmptyState>
      <p>{t(`record.gone.${kind}`)}</p>
      <Button variant="link" onClick={() => navigate({ screen: list })}>
        {isMessageKey(name)
          ? t("shell.navBackTo", { name: t(name) })
          : t("shell.navBackApp")}
      </Button>
    </EmptyState>
  );
}
