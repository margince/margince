// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { worklistKey } from "../../lib/workkeys";
import { readRelationships } from "../../lib/worklistday";
import { QuietDismissAll } from "./worklist.dismissall";
import type { Worklist, WorklistScope } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";
import { rowIdentity } from "./worklist.rowidentity";

type Choices = Pick<
  Worklist,
  "optional_relationships" | "optional_relationships_more_available"
>;

// A day read omits reconnections: working out who went quiet is the costliest
// part of the day, and most readers never open this panel. Opening it reads them.
export function OptionalRelationships({
  day,
  owner,
  scope,
}: Readonly<{ day: Choices; owner: string; scope: WorklistScope }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const carried = day.optional_relationships !== undefined;
  const asked = useQuery({
    queryKey: [...worklistKey, "relationships", scope, owner],
    queryFn: ({ signal }) =>
      readRelationships(scope, owner === "" ? undefined : owner, signal),
    enabled: open && !carried,
  });
  if (carried) {
    return <RelationshipChoices choices={day} owner={owner} />;
  }
  const opener = !open && (
    <Button onClick={() => setOpen(true)}>
      {t("worklist.relationships.show")}
    </Button>
  );
  const note = !open
    ? null
    : asked.isError
      ? "worklist.relationships.failed"
      : asked.isPending
        ? "worklist.relationships.loading"
        : "worklist.relationships.none";
  if (
    open &&
    (asked.data?.optional_relationships?.length ||
      asked.data?.optional_relationships_more_available)
  ) {
    return <RelationshipChoices choices={asked.data} owner={owner} />;
  }
  return (
    <Panel title={t("worklist.relationships.optional")} titleAction={opener}>
      {note && (
        <PanelBody>
          <p className="t-caption">{t(note)}</p>
        </PanelBody>
      )}
    </Panel>
  );
}

function RelationshipChoices({
  choices,
  owner,
}: Readonly<{ choices: Choices; owner: string }>) {
  const t = useT();
  if (
    !choices.optional_relationships?.length &&
    !choices.optional_relationships_more_available
  )
    return null;
  return (
    <Panel
      title={t("worklist.relationships.optional")}
      titleAction={
        <QuietDismissAll rows={choices.optional_relationships ?? []} />
      }
    >
      {choices.optional_relationships?.map((item) => (
        <WorklistRow
          key={rowIdentity(item)}
          item={item}
          density="compact"
          owner={owner}
        />
      ))}
      {choices.optional_relationships_more_available && (
        <PanelBody>
          <p className="t-caption">{t("worklist.relationships.more")}</p>
        </PanelBody>
      )}
    </Panel>
  );
}
