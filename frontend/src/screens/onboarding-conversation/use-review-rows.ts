import { useMemo } from "react";
import type { components } from "../../api/schema";
import type { useT } from "../../i18n";
import type { CompanyDraft, CompanyFieldName } from "../onboarding";
import { isRequired } from "../onboarding";
import type { ClarifyAnswer } from "./company-proposal";
import {
  evidencedFields,
  isCompanyField,
  proposalFromRead,
} from "./company-proposal";
import {
  isWork,
  type ReviewRow,
  reviewFields,
  rowFor,
} from "./company-review-state";
import { deckCards } from "./review-deck";

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type Proposal = components["schemas"]["OnboardingCompanyProposal"];

// The one test for "does this row stop the human continuing".
// `confirmCompanySiteRead` 422s when a required field is still empty,
// so the check goes through `isRequired` rather than the row's state name.
function blocksConfirm(row: ReviewRow): boolean {
  return isRequired(row.field) && row.value.trim() === "";
}

type ReviewRowsArgs = Readonly<{
  proposal: Proposal | undefined;
  read: CompanySiteRead | null;
  draft: CompanyDraft;
  pendingId: string | null;
  answers: readonly ClarifyAnswer[];
  t: ReturnType<typeof useT>;
}>;

/**
 * The review board's rows, and every count the act's surfaces take from them.
 *
 * The deck, the article, the gate and the rail all read this one list. It is
 * built through the `rowFor`/`isWork` pair confirm-card.tsx counts with, so no
 * two surfaces can describe one field differently.
 */
export function useReviewRows({
  proposal,
  read,
  draft,
  pendingId,
  answers,
  t,
}: ReviewRowsArgs) {
  // The review renders even when the proposal endpoint failed: the read
  // snapshot carries the same evidence-gated mapping, without open questions.
  const reviewProposal = useMemo(() => {
    if (proposal) {
      return proposal;
    }
    if (read && (read.status === "ready" || read.status === "partial")) {
      return proposalFromRead(read);
    }
    return null;
  }, [proposal, read]);

  const allRows = useMemo<readonly ReviewRow[]>(() => {
    if (reviewProposal === null) {
      return [];
    }
    const byName = new Map(
      evidencedFields(reviewProposal.fields)
        .filter((field) => isCompanyField(field.field, draft.values))
        .map((field) => [field.field, field]),
    );
    return reviewFields().map((field) => rowFor(field, draft, byName, t));
  }, [reviewProposal, draft, t]);
  const attentionRows = useMemo<readonly ReviewRow[]>(
    () => allRows.filter((row) => isWork(row.state)),
    [allRows],
  );
  // Only an empty required field stops the human; every other outstanding row
  // is advisory.
  const blocking = attentionRows.filter(blocksConfirm);
  const advisory = attentionRows.filter((row) => !blocksConfirm(row));
  // The review card's own "still open" list, from the same inputs it counts.
  const openQuestions = (reviewProposal?.open_questions ?? []).filter(
    (question) =>
      question.id !== pendingId &&
      !answers.some((answer) => answer.clarifyId === question.id),
  );

  return {
    reviewProposal,
    allRows,
    // Undefined while there is no proposal, which is "not known yet" rather
    // than "nothing to ask about".
    uncertainCount:
      reviewProposal === null ? undefined : blocking.length + advisory.length,
    openQuestionCount: openQuestions.length,
    // What the rail names when Confirm is pressed early: the required fields
    // still empty. An open question never holds the reader here.
    confirmBlockers: blocking.map((row) => row.label),
    cards: deckCards(blocking, advisory),
    // Every row, so the deck can still draw the card it stands on after that
    // field stops being outstanding.
    cardOf: (field: CompanyFieldName) =>
      deckCards(
        allRows.filter((row) => row.field === field),
        [],
      )[0],
  };
}
