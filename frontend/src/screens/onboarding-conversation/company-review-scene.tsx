import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { CompanyDraft, CompanyFieldName } from "../onboarding";
import type { ArtifactMode } from "./artifact";
import type { ClarifyAnswer } from "./company-proposal";
import { legalEntityForOption } from "./company-proposal";
import type { ReviewRow } from "./company-review-state";
import { CompanyConfirmCard } from "./confirm-card";
import type { ConversationState } from "./conversation-machine";
import type { CandidateFacts } from "./decision-scene";
import { DecisionScene } from "./decision-scene";
import { ProfileDigest } from "./profile-digest";
import type { DeckCard } from "./review-deck";
import { ReviewDeck } from "./review-deck";
import { WayOnward } from "./way-onward";

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type Proposal = components["schemas"]["OnboardingCompanyProposal"];
type LegalEntity = components["schemas"]["CompanySiteReadLegalEntity"];
type PendingQuestion = NonNullable<ConversationState["pendingQuestion"]>;

// A legal-entity option resolved back to its candidate through the matcher the
// pick uses. A card then never shows details for a candidate the pick would
// fail to find. Any other clarify renders name-only cards.
function entityFacts(
  entities: readonly LegalEntity[],
  value: string,
): CandidateFacts | null {
  const entity = legalEntityForOption(entities, value);
  return entity === undefined
    ? null
    : {
        meta: entity.registered_address,
        // One slot for a number: the registry identity, else the tax one. An
        // entity whose notice printed only a VAT ID is then not a bare name.
        identifier: entity.register_number ?? entity.vat_number,
        snippet: entity.evidence_snippet,
        source: entity.source_url,
      };
}

/** A pending decision, which owns the whole surface while it stands. */
export function CompanyDecision({
  question,
  read,
  onAnswer,
  onDismiss,
}: Readonly<{
  question: PendingQuestion;
  read: CompanySiteRead | null;
  onAnswer: (questionId: string, value: string) => void;
  onDismiss: (questionId: string) => void;
}>) {
  return (
    <DecisionScene
      question={question}
      onAnswer={onAnswer}
      onDismiss={onDismiss}
      factsOf={(value) => entityFacts(read?.legal_entities ?? [], value)}
    />
  );
}

type Identity = Readonly<{
  rootUrl: string;
  logoUrl?: string;
  companyId?: string;
}>;

type ConfirmControl = Readonly<{
  run: () => void;
  pending: boolean;
  blockers: readonly string[];
  held: boolean;
}>;

type ReviewSceneProps = Readonly<{
  mode: ArtifactMode;
  onSwitchMode: (mode: ArtifactMode) => void;
  onReadWhole: () => void;
  onSettle: (field: CompanyFieldName) => void;
  goToField: CompanyFieldName | null;
  proposal: Proposal;
  read: CompanySiteRead | null;
  companyId: string | undefined;
  draft: CompanyDraft;
  setField: (field: CompanyFieldName, value: string) => void;
  rows: Readonly<{
    all: readonly ReviewRow[];
    cards: readonly DeckCard[];
    cardOf: (field: CompanyFieldName) => DeckCard | undefined;
    openQuestions: number;
  }>;
  answers: readonly ClarifyAnswer[];
  missing: readonly CompanyFieldName[];
  selectedFactKeys: string[];
  setSelectedFactKeys: (keys: string[]) => void;
  confirm: ConfirmControl;
  authorizing: boolean;
  error: string | null;
}>;

/**
 * The confirm stop: a deck by default, the whole record on ask, and the fact
 * board behind it.
 *
 * The deck is the front door because the read already knows which fields it
 * could not settle. The whole record stays one press away, where a field is
 * edited freely and a fact is unticked.
 */
export function CompanyReviewScene(props: ReviewSceneProps) {
  // The mark at the head of the record: the read's site, its logo, and the
  // company once one exists. Undefined before a read draws the monogram.
  const identity: Identity | undefined =
    props.read === null
      ? undefined
      : {
          rootUrl: props.read.root_url,
          logoUrl: props.read.logo_url,
          companyId: props.companyId,
        };
  if (props.mode === "dossier") {
    return <DeckScene {...props} identity={identity} />;
  }
  if (props.mode === "profile") {
    return <WholeRecordScene {...props} identity={identity} />;
  }
  return <FactBoardScene {...props} />;
}

type SceneWithIdentity = ReviewSceneProps &
  Readonly<{ identity: Identity | undefined }>;

function DeckScene({
  rows,
  selectedFactKeys,
  setField,
  confirm,
  onReadWhole,
  goToField,
  identity,
}: SceneWithIdentity) {
  return (
    <ReviewDeck
      cards={rows.cards}
      cardOf={rows.cardOf}
      settled={selectedFactKeys.length}
      onField={setField}
      onDone={confirm.run}
      blockers={confirm.blockers}
      held={confirm.held}
      openQuestions={rows.openQuestions}
      // Every row: the article is what the record says, not the deck's list
      // again in prose.
      digest={(active) => (
        <ProfileDigest
          rows={rows.all}
          active={active}
          identity={identity}
          onReadWhole={onReadWhole}
        />
      )}
      pending={confirm.pending}
      goTo={goToField ?? undefined}
    />
  );
}

// The whole record as a two-column document. A reader who asks to see
// everything is reading, so this door shows prose rather than controls.
function WholeRecordScene({
  rows,
  read,
  identity,
  onSettle,
  setField,
  onSwitchMode,
  draft,
  confirm,
}: SceneWithIdentity) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <div className="ob-scene">
      {/* The way back to the deck, whose tray and open count are otherwise
          unreachable from here. */}
      <Button variant="ghost" onClick={() => onSwitchMode("dossier")}>
        {t("ob.deck.backToOpen")}
      </Button>
      {/* `read` is passed only here: the reader asked for the whole record,
          and the crawl found far more than the deck asks about. `onSettle`
          points the deck back at an unanswered line. */}
      <ProfileDigest
        rows={rows.all}
        read={read ?? undefined}
        identity={identity}
        onSettle={onSettle}
        onField={setField}
      />
      {/* The board is where a fact is ticked, so its door is named for that. */}
      <Button variant="ghost" onClick={() => onSwitchMode("record")}>
        {t("ob.digest.pickFacts")}
      </Button>
      {/* One Save for every corrected line, shown once one is: an untouched
          record already has the deck's Confirm. */}
      {draft.edited.size > 0 && (
        <WayOnward
          label={t("ob.digest.saveChanges")}
          pendingLabel={t("ob.s1.saving")}
          pending={confirm.pending}
          blockers={confirm.blockers}
          held={confirm.held}
          stillNeeded={(fields) =>
            t("ob.deck.stillNeeded", { fields: fields.join(", ") })
          }
          note={
            <p className="ob-stage-hint">
              {plural("ob.digest.changed", draft.edited.size, {
                count: formatNumber(draft.edited.size, locale),
              })}
            </p>
          }
          onGo={confirm.run}
        />
      )}
    </div>
  );
}

function FactBoardScene({
  onSwitchMode,
  companyId,
  proposal,
  draft,
  answers,
  read,
  selectedFactKeys,
  setSelectedFactKeys,
  missing,
  setField,
  confirm,
  authorizing,
  error,
}: ReviewSceneProps) {
  const t = useT();
  return (
    <div className="ob-scene">
      <Button variant="ghost" onClick={() => onSwitchMode("profile")}>
        {t("ob.deck.backToRecord")}
      </Button>
      <CompanyConfirmCard
        companyId={companyId}
        proposal={proposal}
        draft={draft}
        answers={answers}
        read={read}
        selectedFactKeys={selectedFactKeys}
        setSelectedFactKeys={setSelectedFactKeys}
        missingRequired={missing}
        setField={setField}
        onAcceptAll={confirm.run}
        pending={confirm.pending}
        authorizing={authorizing}
        error={error}
      />
    </div>
  );
}
