import { Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { IconAction } from "../../design-system/iconaction";
import { Panel, PanelBody } from "../../design-system/panel";
import { provenanceOf } from "../../design-system/provenanceof";
import { Row } from "../../design-system/stack";
import { AutonomyDot, ProvenanceTag } from "../../design-system/trust";
import { formatMoney } from "../../format/format";
import { type Locale, useT } from "../../i18n";
import { approvalDotTier, useAgentTierMap } from "../../lib/autonomy";
import { navigate } from "../../lib/router";
import { useViewerId } from "../../lib/session";
import { RecordReading, RecordReadingPair } from "../../screens/record360";
import { approvalKindLabel } from "../automations/approvalkind";
import { DealBrief } from "./deal360/dealbrief";
import { DEAL_OFFERS_ANCHOR } from "./deal360/dealcockpit";
import { DealCommitteeCard } from "./deal360/dealcommitteecard";
import { DealWatchCard } from "./deal360/dealwatchcard";
import { OutcomeReviewPanel } from "./deal360/outcomereview";
import type { useDealCoverage } from "./deal360/usedealcoverage";
import { StartDeliveryPrompt } from "./dealproject";
import type { Deal, Offer } from "./deals";
import { DealStatusCardPanel } from "./dealstatus";

type Approval = components["schemas"]["Approval"];

// The live 🟡 confirm-first staging queue for this deal, kept out of
// DealScreen's render for readability, as DealActions is
// (deal360/dealactions.tsx).
function DealApprovals({
  approvals,
  decide,
}: Readonly<{
  approvals: Approval[];
  decide: (input: {
    approvalId: string;
    verdict: "approve" | "reject";
  }) => void;
}>) {
  const t = useT();
  const tierMap = useAgentTierMap();
  const viewerId = useViewerId();
  if (approvals.length === 0) {
    return null;
  }
  return (
    <Panel title={t("deal.pendingApprovals")}>
      {/* The interval between two staged cards belongs to the stack rather
          than to each card, so the last one does not pay for a neighbour it
          does not have. */}
      <PanelBody className="form-stack">
        {approvals.map((approval) => (
          <div key={approval.id} className="staging-card">
            <Row gap="2" wrap={false}>
              <AutonomyDot tier={approvalDotTier(approval.kind, tierMap)} />
              {/* The same two facts the approvals inbox states, said the same
                way. Printed off the wire they read `advance_deal` and
                `agent:capture`: the vocabulary the API speaks, on a page
                whose reader never sees the API. */}
              <span className="t-label">
                {approvalKindLabel(approval.kind, t)}
              </span>
              <ProvenanceTag
                provenance={provenanceOf(approval.proposed_by, viewerId)}
              />
            </Row>
            <ActionRow
              className="approval-gate"
              primary={
                <Button
                  variant="primary"
                  onClick={() =>
                    decide({ approvalId: approval.id, verdict: "approve" })
                  }
                >
                  {t("trust.accept")}
                </Button>
              }
            >
              {/* Dismiss here sends the `reject` verdict, the same answer the
                decision card's trash can gives, so it wears the same glyph. */}
              <IconAction
                label={t("trust.dismiss")}
                icon={<Trash2 aria-hidden />}
                onClick={() =>
                  decide({ approvalId: approval.id, verdict: "reject" })
                }
              />
            </ActionRow>
          </div>
        ))}
      </PanelBody>
    </Panel>
  );
}

// Exported so a render test can reach the refusal state directly. It reads the
// system-of-record mode, so unlike FxLine it needs a query client around it.
// Whether the New-offer control is refused depends on its props alone.
export function OffersPanel({
  offers,
  creating,
  locale,
  dealCurrency,
  refusedReasonId,
  onCreate,
}: Readonly<{
  offers: Offer[] | undefined;
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
  return (
    <Panel
      title={t("deal.offers")}
      // The panel's one verb, in the header band beside the title. A single
      // button that opens what the panel lists belongs to the panel's name,
      // not to a strip under its rows.
      titleAction={
        <Button
          // `reason` disables the control and points at the explanation. Passing
          // `disabled` beside it would cancel the refusal it sets, so the
          // in-flight case stays on `disabled` and the state case on `reason`.
          // An empty code is as absent as a null one: `formatMoneyOrAbsent`
          // treats it that way, and Intl throws on it.
          disabled={Boolean(dealCurrency) && creating}
          reasonId={refusedReasonId}
          reason={dealCurrency ? undefined : t("deal.offerNeedsCurrency")}
          onClick={() => {
            if (dealCurrency) {
              onCreate(dealCurrency);
            }
          }}
        >
          {t("deal.newOffer")}
        </Button>
      }
    >
      <PanelBody>
        {offers &&
          (offers.length > 0 ? (
            <DataTable
              label={t("deal.offers")}
              columns={[
                {
                  key: "offer_number",
                  header: t("deal.offerNumber"),
                  render: (offer: Offer) => offer.offer_number,
                },
                {
                  key: "revision",
                  header: t("deal.offerRevision"),
                  render: (offer: Offer) => String(offer.revision),
                },
                {
                  key: "status",
                  header: t("lead.status"),
                  render: (offer: Offer) => <Badge>{offer.status}</Badge>,
                },
                {
                  key: "gross",
                  header: t("deals.amount"),
                  render: (offer: Offer) => (
                    <span className="t-num">
                      {formatMoney(offer.gross_minor, offer.currency, locale)}
                    </span>
                  ),
                },
              ]}
              rows={offers}
              rowKey={(offer) => offer.id}
              onRowClick={(offer) =>
                navigate({ screen: "offers", id: offer.id })
              }
            />
          ) : (
            <EmptyState>{t("deal.offersEmpty")}</EmptyState>
          ))}
      </PanelBody>
    </Panel>
  );
}

// The deal 360's overview pane, apart from DealScreen so the tab switch does
// not push the screen's render closure over the cognitive-complexity budget.
export function DealOverviewPane({
  deal,
  dealApprovals,
  onDecide,
  offers,
  creatingOffer,
  locale,
  onCreateOffer,
  readOnly,
  refusedReasonId,
  pulse,
  spine,
  path,
  coverage,
  onOpenEmail,
}: Readonly<{
  deal: Deal;
  dealApprovals: Approval[];
  onDecide: (input: {
    approvalId: string;
    verdict: "approve" | "reject";
  }) => void;
  offers: Offer[] | undefined;
  creatingOffer: boolean;
  locale: Locale;
  onCreateOffer: (currency: string) => void;
  /** Whether the deal takes no field write at all: archived, or not this
   * caller's to write. This is not `advanceRefused`, which also refuses a
   * closed deal. Reopening is its own action, while correcting the brief of a
   * deal won last week is an ordinary edit the server accepts. Merging the two
   * would hide the brief control on every closed deal. */
  readOnly: boolean;
  /** The id of the page's sentence about why this deal takes no changes, or
   * undefined while it does. An offer is hung off the deal, so a deal this
   * caller cannot write takes no new offer from them either. */
  refusedReasonId?: string;
  // Whose move it is, in one sentence, drawn under the call inside the
  // reading. It is the sentence the call rests on, not a header fact.
  pulse: ReactNode | undefined;
  // The deal's story as a thread, drawn under the call inside the reading.
  spine: ReactNode;
  // Where the deal stands on its pipeline, inside the reading.
  path?: ReactNode;
  // Who is on the deal, for the reading's reference pair beside the offers.
  coverage: ReturnType<typeof useDealCoverage>;
  // Opens a cited message in the page's own email drawer; see `Citations`.
  onOpenEmail?: (activityId: string) => void;
}>) {
  return (
    // The same stack every record's overview reads down, with its rhythm.
    <div className="record-stack record-sections">
      {/* A won deal with no project, on a company with one open project, is
          offered that project once. Nothing else here asks. */}
      <StartDeliveryPrompt deal={deal} />
      {/* One reading, in parts: the call with the deal's thread, the move the
          briefing names, then what is on the table beside who is in the room
          (the deal's two sides), then what the customer committed to. */}
      <RecordReading>
        <DealStatusCardPanel
          dealId={deal.id}
          dealName={deal.name}
          pulse={pulse}
          spine={spine}
          path={path}
          onOpenEmail={onOpenEmail}
        />
        <DealApprovals approvals={dealApprovals} decide={onDecide} />
        <RecordReadingPair>
          {/* The money reading's door scrolls here; the id is the strip's own
              constant, so a rename cannot leave the door pointing at nothing. */}
          <div id={DEAL_OFFERS_ANCHOR}>
            <OffersPanel
              offers={offers}
              creating={creatingOffer}
              locale={locale}
              dealCurrency={deal.currency ?? null}
              refusedReasonId={refusedReasonId}
              onCreate={onCreateOffer}
            />
          </div>
          <DealCommitteeCard
            dealId={deal.id}
            coverage={coverage.coverage}
            withheld={coverage.withheld}
            pending={coverage.pending}
            refusedReasonId={refusedReasonId}
          />
        </RecordReadingPair>
        <DealWatchCard dealId={deal.id} onOpenEmail={onOpenEmail} />
      </RecordReading>
      <DealBrief
        brief={deal.description}
        // The record's own write refusal, not the advance verb's. A closed
        // deal takes no stage move without a reopen, but takes an ordinary
        // field edit. The server asks only for update permission, row
        // writability and an unarchived record.
        readOnly={readOnly}
      />
      {/* Under the brief, and only on a closed deal: the panel returns null
          while the deal is still open, because there is no outcome to review
          yet and inviting one would be asking for a verdict nobody can give. */}
      <OutcomeReviewPanel
        dealId={deal.id}
        status={deal.status}
        closingOccurrenceId={deal.closing_occurrence_id}
      />
      {/* Neither the related-evidence panel nor the inline log form stands
          here any more. The evidence it listed — recent touches, open tasks,
          the contacts and companies around the deal — is already the reading
          above and the rail beside it, and a reader given the same records
          twice has two lists to reconcile. Logging is a verb, and the head
          carries it (dealactions.tsx): a form always open at the foot of the
          page asks for a note on every read. */}
    </div>
  );
}
