import { useId } from "react";
import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../design-system/overviewcard";
import { type Locale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { tasksPeek } from "../../lib/standingpeeks";
import { dealRoleLabel } from "../records/record360";
import { DealApprovals } from "./deal360/dealapprovals";
import { DealBrief } from "./deal360/dealbrief";
import { DealCommitteeCard } from "./deal360/dealcommitteecard";
import { DealHowItStands } from "./deal360/dealhowitstands";
import { DealLinksCard } from "./deal360/deallinkscard";
import { offerRow, otherOpenTaskRows } from "./deal360/dealoverviewrows";
import { DealRoomCard } from "./deal360/dealroomcard";
import type { DealStandingInputs } from "./deal360/dealstanding";
import type { Offer } from "./deals";
import { DealStatusCardPanel, unreadHead } from "./dealstatus";
import "./deal360/deal360.css";

type Approval = components["schemas"]["Approval"];

// Exported so a render test can reach the refusal state directly. Whether the
// New-offer link is refused depends on its props alone.
export function OffersPanel({
  offers,
  creating,
  locale,
  dealCurrency,
  refusedReasonId,
  onCreate,
}: Readonly<{
  offers: readonly Offer[] | undefined;
  creating: boolean;
  locale: Locale;
  // An offer is written in the deal's currency, so a deal nobody has priced
  // has nothing to write one in. Null refuses the control and says why. It
  // never creates an offer in a currency the code chose.
  dealCurrency: string | null;
  // The id of the page's sentence about why the deal takes no changes, when
  // it does not. The server hangs a new offer off the deal through the deal's
  // own write gate. So the control is refused by the same fact as Edit.
  refusedReasonId?: string;
  onCreate: (currency: string) => void;
}>) {
  const t = useT();
  const zone = useRecordZone();
  const currencyReasonId = useId();
  // The sentence for an unpriced deal closes the card, so the link keeps its
  // place beside the title; the refused link points at it.
  const needsCurrency = !dealCurrency;
  return (
    <OverviewCard
      title={t("deal.offers")}
      action={
        <Button
          variant="link"
          // A refused control points at its reason. Passing `disabled` beside
          // it would cancel the refusal, so only the in-flight case uses it.
          // An empty code is as absent as a null one: Intl throws on it.
          disabled={!needsCurrency && creating}
          reasonId={
            refusedReasonId ?? (needsCurrency ? currencyReasonId : undefined)
          }
          onClick={() => {
            if (dealCurrency) {
              onCreate(dealCurrency);
            }
          }}
        >
          {t("deal.newOffer")}
        </Button>
      }
      empty={offers ? t("deal.offersEmpty") : undefined}
      foot={
        needsCurrency ? (
          <span id={currencyReasonId}>{t("deal.offerNeedsCurrency")}</span>
        ) : undefined
      }
    >
      {offers && offers.length > 0 && (
        <OverviewList>
          {offers.map((offer) => {
            const row = offerRow(offer, t, locale, zone);
            return (
              <OverviewRow
                key={row.id}
                title={
                  <button
                    type="button"
                    className="deal-offer-open"
                    onClick={() => navigate({ screen: "offers", id: row.id })}
                  >
                    {row.title}
                  </button>
                }
                sub={row.sub}
                right={<Badge>{row.statusLabel}</Badge>}
              />
            );
          })}
        </OverviewList>
      )}
    </OverviewCard>
  );
}

// The deal's Overview: the next move, How it stands, then the cards in the
// design's two columns. Apart from DealScreen so the tab switch does not push
// the screen's render closure over the cognitive-complexity budget.
export function DealOverviewPane({
  inputs,
  creatingOffer,
  locale,
  onCreateOffer,
  readOnly,
  refusedReasonId,
  onOpenEmail,
  onOpenTab,
  onEditDetails,
  threadComplete,
  dealApprovals,
  onDecide,
}: Readonly<{
  // What How it stands reads, and the ladder's write path.
  inputs: DealStandingInputs;
  creatingOffer: boolean;
  locale: Locale;
  onCreateOffer: (currency: string) => void;
  /** Whether the deal takes no field write at all: archived, or not this
   * caller's to write. This is not `advanceRefused`, which also refuses a
   * closed deal: correcting the brief of a deal won last week is an ordinary
   * edit the server accepts. */
  readOnly: boolean;
  /** The id of the page's sentence about why this deal takes no changes, or
   * undefined while it does. */
  refusedReasonId?: string;
  // Opens a cited message in the page's own email drawer; see `Citations`.
  onOpenEmail?: (activityId: string) => void;
  // Switches the page to another of the deal's tabs.
  onOpenTab: (tab: "room" | "history") => void;
  // Opens the drawer that holds the deal's fields.
  onEditDetails: () => void;
  // Whether the thread read holds every row. A count of tasks off a page
  // with more behind it would be a floor the bar cannot mark. So it is left
  // out until then.
  threadComplete: boolean;
  // The deal's pending approvals, answered in place under How it stands.
  dealApprovals: Approval[];
  onDecide: (input: {
    approvalId: string;
    verdict: "approve" | "reject";
  }) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const zone = useRecordZone();
  const { deal, offers, coverage, activities, card } = inputs;
  // The task the bar names is not one of the others.
  const shown = (card?.next?.evidence ?? []).flatMap((row) =>
    row.activity_id ? [row.activity_id] : [],
  );
  const others = otherOpenTaskRows(activities, shown);
  const more = threadComplete
    ? {
        count: others.length,
        onOpen: () => onOpenTab("history"),
        peek: tasksPeek(
          others.map((row) => ({
            id: row.id,
            subject: row.subject ?? t("task.untitled"),
            dueAt: row.due_at,
            overdue: row.due_at
              ? Date.parse(row.due_at) < Date.parse(inputs.asOf)
              : false,
          })),
          { t, plural, locale, zone },
          others.length,
          () => onOpenTab("history"),
        ),
      }
    : undefined;
  return (
    <>
      {/* The server refuses an archived deal's status card, so its reading
          is the records' own and nothing model-written is claimed. */}
      {deal.archived_at ? (
        <DealHowItStands
          head={unreadHead(deal, t)}
          inputs={inputs}
          onOpenHistory={() => onOpenTab("history")}
        />
      ) : (
        <DealStatusCardPanel
          deal={deal}
          onOpenEmail={onOpenEmail}
          more={more}
          renderStanding={(head) => (
            <DealHowItStands
              head={head}
              inputs={inputs}
              onOpenHistory={() => onOpenTab("history")}
            />
          )}
        />
      )}
      <DealApprovals approvals={dealApprovals} decide={onDecide} />
      <div className="deal-overview-grid">
        <div className="deal-overview-column">
          <OffersPanel
            offers={offers}
            creating={creatingOffer}
            locale={locale}
            dealCurrency={deal.currency ?? null}
            refusedReasonId={refusedReasonId}
            onCreate={onCreateOffer}
          />
          {!deal.archived_at && (
            <DealCommitteeCard
              dealId={deal.id}
              ownerId={deal.owner_id}
              coverage={coverage.coverage}
              withheld={coverage.withheld}
              pending={coverage.pending}
              refusedReasonId={refusedReasonId}
              roleLabel={(role) => dealRoleLabel(role, t)}
            />
          )}
          <DealLinksCard deal={deal} />
        </div>
        <div className="deal-overview-column">
          <DealBrief
            brief={deal.description}
            // The record's own write refusal, not the advance verb's.
            onEdit={readOnly ? undefined : onEditDetails}
          />
          <DealRoomCard dealId={deal.id} onOpen={() => onOpenTab("room")} />
        </div>
      </div>
    </>
  );
}
