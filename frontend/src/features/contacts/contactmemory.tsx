import type { components } from "../../api/schema";
import { useInteractionLabel } from "../../components/interactionchrome";
import { Button } from "../../design-system/atoms";
import { CallAction } from "../../design-system/callaction";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../design-system/overviewcard";
import { useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { omitted } from "./contact360";
import { CardLink, rowPill } from "./contactcards";
import { callBackNumber, memoryRows } from "./contactmemory.rows";

// What was said: conversations as entities, condensed. The History tab stays
// the whole ledger, with its filters, faces and Reply. A summary never
// replaces the original, and a withheld activity never leaks through one.

export type Contact360 = components["schemas"]["Contact360"];

export type Activity = components["schemas"]["Activity"];

// The card is a glance read beside Commitments, so it draws the latest two
// and leaves the rest to Full history.
const GLANCE = 2;

export function ContactMemory({
  view,
  onOpenEmail,
  onFullHistory,
}: Readonly<{
  view: Contact360;
  onOpenEmail: (activityId: string) => void;
  onFullHistory: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // When a conversation happened is a fact about the record, so it reads in
  // the record's zone and two colleagues name the same day.
  const recordZone = useRecordZone();
  const interactionLabel = useInteractionLabel();
  const withheld =
    omitted(view, "activities") && omitted(view, "conversation_memory");
  const rows = withheld
    ? []
    : memoryRows(view, t, interactionLabel, locale, recordZone).slice(
        0,
        GLANCE,
      );
  return (
    <OverviewCard
      title={t("contact.card.said")}
      action={
        <CardLink onPress={onFullHistory}>
          {t("contact.card.fullHistory")}
        </CardLink>
      }
      empty={withheld ? t("state.withheld") : t("contact.card.saidEmpty")}
    >
      {rows.length > 0 && (
        <OverviewList>
          {rows.map((row) => {
            const emailId = row.emailId;
            const number = callBackNumber(row.kind, view.contact);
            return (
              <OverviewRow
                key={row.key}
                title={
                  emailId ? (
                    <Button
                      variant="link"
                      className="pe-card-open"
                      onClick={() => onOpenEmail(emailId)}
                    >
                      {row.title}
                    </Button>
                  ) : (
                    row.title
                  )
                }
                sub={row.sub}
                right={
                  number ? (
                    <>
                      {rowPill(row)}
                      <CallAction
                        appearance="button"
                        label={t("record.verb.call")}
                        number={number}
                        record={{
                          entityType: "contact",
                          entityId: view.contact.id,
                        }}
                      />
                    </>
                  ) : (
                    rowPill(row)
                  )
                }
              />
            );
          })}
        </OverviewList>
      )}
    </OverviewCard>
  );
}
