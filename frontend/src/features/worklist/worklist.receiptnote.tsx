// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { activityTimeline } from "../../design-system/activitytimeline";
import { ProvenanceTag } from "../../design-system/provenance";
import { TimelineBody } from "../../design-system/timelinetext";
import { Row } from "../../design-system/stack";
import { useLocale, useT } from "../../i18n";
import { useActivity } from "../../lib/activityread";
import { useMe } from "../../lib/session";

export function ReceiptNote({
  activityId,
  children,
}: Readonly<{ activityId: string; children: ReactNode }>) {
  const activity = useActivity(activityId).data;
  const me = useMe().data;
  const t = useT();
  const { locale } = useLocale();
  if (
    !activity ||
    activity.kind !== "note" ||
    activity.content_state === "withheld"
  ) {
    return <>{children}</>;
  }
  const [entry] = activityTimeline([activity], me?.user.id, undefined, {
    t,
    locale,
  });
  if (!entry) return <>{children}</>;
  return (
    <div>
      <Row gap="2" wrap>
        {children}
        {entry.provenance?.kind === "agent" && (
          <ProvenanceTag provenance={entry.provenance} />
        )}
      </Row>
      <TimelineBody body={entry.body ?? ""} entry={entry} />
    </div>
  );
}
