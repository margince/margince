// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { RECORD_CHIP_ICONS } from "../../components/recordlinks";
import { RecordCard } from "../../design-system/recordcard";
import { StatusBadge } from "../../design-system/statusbadge";
import { useNow } from "../../format/now";
import { useLocale, useT } from "../../i18n";
import { describeHit } from "../../lib/searchstanding";
import { useViewerZone } from "../../lib/viewerzone";

type SearchResult = components["schemas"]["SearchResult"];

/**
 * A deal, with the stage, amount and close date the hit's standing carries, in
 * the same words the palette gives them. `marks` are the hit's own badges.
 */
export function DealHit({
  hit,
  href,
  marks,
}: Readonly<{ hit: SearchResult; href: string; marks: ReactNode[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const now = new Date(useNow(60_000));
  const { subtitle, status } = describeHit(hit, hit.snippet ?? undefined, {
    t,
    locale,
    zone,
    now,
  });
  const facts = (status || subtitle || marks.length > 0) && (
    <span className="search-hit-marks">
      {status && <StatusBadge kind={status.kind} label={status.label} />}
      {subtitle && <span>{subtitle}</span>}
      {marks}
    </span>
  );
  return (
    <li className="search-hit">
      <RecordCard
        kind="deal"
        icon={RECORD_CHIP_ICONS.deal}
        name={hit.title ?? hit.id}
        identity={hit.id}
        href={href}
        position={facts}
      />
    </li>
  );
}
