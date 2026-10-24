// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../design-system/overviewcard";
import { provenanceOf } from "../../design-system/provenanceof";
import { formatDayMonth } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useMemberName } from "../../lib/membernames";
import { useRecordZone } from "../../lib/recordzone";
import { recordNamesIn } from "./company360";
import { latestExchanges } from "./companyoverviewrows";
import { activityHeadline } from "./companyrecent";
import type { CompanyTab } from "../../lib/companytab";
import "./company360.css";

type Company360 = components["schemas"]["Company360"];
type Activity = NonNullable<Company360["activities"]>["data"][number];

const KIND_WORD: Record<Activity["kind"], MessageKey> = {
  email: "co.overview.kind.email",
  call: "co.overview.kind.call",
  meeting: "co.overview.kind.meeting",
  note: "co.overview.kind.note",
  task: "co.overview.kind.task",
  message: "co.overview.kind.message",
};

/** The newest exchanges on the account, one line each, newest first. */
export function LatestCard({
  view,
  onOpenTab,
  onOpenEmail,
}: Readonly<{
  view: Company360;
  onOpenTab: (tab: CompanyTab) => void;
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const rows = latestExchanges(view);
  const hidden = view.sections_omitted.includes("activities");
  return (
    <OverviewCard
      title={t("co.overview.latest")}
      action={
        <Button variant="link" onClick={() => onOpenTab("timeline")}>
          {t("co.360.fullHistory")}
        </Button>
      }
      empty={t(hidden ? "co.section.restricted" : "co.recent.empty")}
    >
      {rows.length > 0 && (
        <OverviewList>
          {rows.map((activity) => (
            <LatestRow
              key={activity.id}
              activity={activity}
              view={view}
              onOpenEmail={onOpenEmail}
            />
          ))}
        </OverviewList>
      )}
    </OverviewCard>
  );
}

function LatestRow({
  activity,
  view,
  onOpenEmail,
}: Readonly<{
  activity: Activity;
  view: Company360;
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const who = useWhoOf(activity, view);
  const title = activityHeadline(activity, t);
  const email = activity.email_summary;
  const sub = [
    who,
    t(KIND_WORD[activity.kind]),
    formatDayMonth(activity.occurred_at, locale, zone),
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <OverviewRow
      title={
        email ? (
          <button
            type="button"
            className="co-rowlink"
            onClick={() => onOpenEmail(activity.id)}
          >
            {title}
          </button>
        ) : (
          title
        )
      }
      sub={sub}
      right={
        email?.move === "needs_reply" && (
          <Badge tone="warning">{t("co.strip.unanswered")}</Badge>
        )
      }
    />
  );
}

// Who the exchange was with or by. That is the other side of an email, else
// the colleague who recorded it, else the contact it is linked to.
function useWhoOf(activity: Activity, view: Company360): string | undefined {
  const nameOf = recordNamesIn(view);
  const email = activity.email_summary;
  const provenance = provenanceOf(activity.captured_by);
  const member = useMemberName(
    !email && provenance.kind === "human" ? provenance.userId : undefined,
  );
  const counterparty =
    (email?.counterparty_contact_id &&
      nameOf("contact", email.counterparty_contact_id)) ||
    email?.counterparty ||
    member.data;
  if (counterparty) {
    return counterparty;
  }
  const linked = activity.links?.find((link) => link.entity_type === "contact");
  return linked ? nameOf("contact", linked.entity_id) : undefined;
}
