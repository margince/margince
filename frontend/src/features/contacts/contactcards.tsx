import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Badge, Button, Checkbox } from "../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../design-system/overviewcard";
import { Panel, PanelRow } from "../../design-system/panel";
import { badgeTone } from "../../design-system/readingtone";
import { useNow } from "../../format/now";
import { useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { DismissClaimButton } from "../records/claimdismiss";
import { TakenOverLine, TakeOverPromiseButton } from "./claimtakeover";
import { omitted } from "./contact360";
import { ContactClaimsReview, claimsForReview } from "./contactclaimsreview";
import {
  commitmentRows,
  dealCard,
  type OverviewRow as Row,
  understandingRows,
} from "./contactoverview.rows";

// Three of the overview's four cards; What was said is contactmemory.tsx.
// Each reads what the 360 already assembled, so none can show a record the
// page beside it is withholding.

type Contact360 = components["schemas"]["Contact360"];

export function ContactCommitmentsCard({
  view,
  firstName,
  onAllTasks,
}: Readonly<{ view: Contact360; firstName: string; onAllTasks: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const nowMs = useNow(60_000);
  const withheld = omitted(view, "claims");
  const rows = withheld
    ? []
    : commitmentRows(view, firstName, nowMs, t, locale);
  const claims = withheld ? [] : (view.claims ?? []);
  const hasReview = claimsForReview(claims).length > 0;
  return (
    <OverviewCard
      title={t("record.commitments")}
      action={<CardLink onPress={onAllTasks}>{t("record.allTasks")}</CardLink>}
      // A card with nothing promised on a record whose mail holds no promise
      // is correct, not a gap.
      empty={withheld ? t("state.withheld") : t("contact.loops.empty")}
    >
      {(rows.length > 0 || hasReview) && (
        <>
          {rows.length > 0 && (
            <OverviewList>
              {rows.map((row) => (
                <OverviewRow
                  key={row.key}
                  // A read of the promise's done state, never a write.
                  lead={
                    <Checkbox
                      label={<span className="sr-only">{row.title}</span>}
                      checked={row.tick === true}
                      disabled
                    />
                  }
                  title={row.title}
                  sub={loopSub(row, view.contact.id)}
                  right={rowPill(row)}
                />
              ))}
            </OverviewList>
          )}
          <ContactClaimsReview
            claims={claims}
            dismiss={(id) => <DismissClaimButton id={id} />}
          />
        </>
      )}
    </OverviewCard>
  );
}

// The line under a promise, and below it who took the promise over or the
// verb that lets the reader take it.
function loopSub(row: Row, contactId: string): ReactNode {
  if (!row.takenBy && !row.takeable) {
    return row.sub;
  }
  return (
    <>
      {row.sub}
      {row.takenBy && <TakenOverLine takenBy={row.takenBy} />}
      {row.takeable && (
        <TakeOverPromiseButton id={row.key} contactId={contactId} />
      )}
    </>
  );
}

export function ContactDealCard({
  view,
  firstName,
  onOpenDeals,
}: Readonly<{ view: Contact360; firstName: string; onOpenDeals: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const card = dealCard(view, t, locale, recordZone);
  const title = t("contact.card.dealTitle", { name: firstName });
  if (card.state !== "deal") {
    return (
      <OverviewCard
        title={title}
        action={
          card.state === "none" && (
            <CardLink onPress={onOpenDeals}>{t("contact.card.deals")}</CardLink>
          )
        }
        empty={
          card.state === "withheld"
            ? t("contact.commercial.withheld")
            : t("contact.commercial.noDeal")
        }
      />
    );
  }
  return (
    <OverviewCard
      title={title}
      action={
        <CardLink onPress={onOpenDeals}>{t("contact.card.deals")}</CardLink>
      }
      foot={card.foot.text}
      footTone={card.foot.tone === "warning" ? "warn" : undefined}
    >
      <OverviewList>
        <OverviewRow
          title={
            <Button
              variant="link"
              className="pe-card-open"
              onClick={() => navigate({ screen: "deals", id: card.dealId })}
            >
              {card.row.title}
            </Button>
          }
          sub={card.row.sub}
          right={card.row.value}
          figure
        />
      </OverviewList>
    </OverviewCard>
  );
}

export function ContactUnderstandingCard({
  view,
  firstName,
  onCorrect,
}: Readonly<{ view: Contact360; firstName: string; onCorrect: () => void }>) {
  const t = useT();
  const rows = understandingRows(view, t);
  return (
    <OverviewCard
      title={t("contact.card.understandingTitle", { name: firstName })}
      action={
        rows && (
          <CardLink onPress={onCorrect}>{t("contact.card.correct")}</CardLink>
        )
      }
      empty={t("state.withheld")}
    >
      {rows && (
        <OverviewList>
          {rows.map((row) => (
            <OverviewRow key={row.key} title={row.title} sub={row.sub} />
          ))}
        </OverviewList>
      )}
    </OverviewCard>
  );
}

/**
 * Everything the extractor read out of this contact's conversations that no
 * one has settled. Each reading sits beside the words it came from, with a way
 * to dismiss one that got it wrong. The Research tab draws it, where
 * Understanding's "Correct something" leads.
 */
export function ContactClaimReadings({ view }: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const open = omitted(view, "claims")
    ? []
    : (view.claims ?? []).filter((claim) => claim.status === "open");
  if (open.length === 0) {
    return null;
  }
  return (
    <Panel title={t("contact.research.readings")}>
      {open.map((claim) => (
        <PanelRow className="pe-loop" key={claim.id}>
          <span className="pe-loop-body">
            {claim.body}
            {claim.source_quote && (
              <span className="pe-loop-quote t-caption">
                {t("commitment.quote", { quote: claim.source_quote })}
              </span>
            )}
          </span>
          {/* A promise a colleague took over was made; it is theirs now. */}
          {!claim.taken_at && <DismissClaimButton id={claim.id} />}
        </PanelRow>
      ))}
    </Panel>
  );
}

/** The card's one way out, at the end of its title line. */
export function CardLink({
  onPress,
  children,
}: Readonly<{ onPress: () => void; children: string }>) {
  return (
    <Button variant="link" className="pe-card-link" onClick={onPress}>
      {children}
    </Button>
  );
}

/** The row's pill at its right, or nothing when the row has none. */
export function rowPill(row: Row): ReactNode {
  return (
    row.pill && <Badge tone={badgeTone(row.pill.tone)}>{row.pill.text}</Badge>
  );
}
