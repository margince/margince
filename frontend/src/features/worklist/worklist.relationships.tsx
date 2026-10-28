// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import type { Worklist } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";
import { rowIdentity } from "./worklist.rowidentity";

export function OptionalRelationships({
  day,
  owner,
}: Readonly<{
  day: Pick<
    Worklist,
    "optional_relationships" | "optional_relationships_more_available"
  >;
  owner: string;
}>) {
  const t = useT();
  if (
    !day.optional_relationships?.length &&
    !day.optional_relationships_more_available
  )
    return null;
  return (
    <Panel title={t("worklist.relationships.optional")}>
      <PanelBody>
        <p className="t-caption">
          {t("worklist.relationships.optionalDetail")}
        </p>
      </PanelBody>
      {day.optional_relationships?.map((item) => (
        <WorklistRow
          key={rowIdentity(item)}
          item={item}
          density="compact"
          owner={owner}
        />
      ))}
      {day.optional_relationships_more_available && (
        <PanelBody>
          <p className="t-caption">{t("worklist.relationships.more")}</p>
        </PanelBody>
      )}
    </Panel>
  );
}
