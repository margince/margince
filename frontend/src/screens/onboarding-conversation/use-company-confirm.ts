import type { UseQueryResult } from "@tanstack/react-query";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { Dispatch } from "react";
import { useCallback } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { useT } from "../../i18n";
import { problemMessageOf, throwProblem } from "../common";
import { storeCompany } from "../installationcompany";
import type { CompanyDraft, CompanyForm } from "../onboarding";
import type { ConfirmRefusal } from "./artifact";
import type { ClarifyAnswer } from "./company-proposal";
import { proposalFromRead, resolutionsFromAnswers } from "./company-proposal";
import type { ConversationEvent } from "./conversation-machine";
import { useConfirmRecovery } from "./use-confirm-recovery";
import type { WizardPersistInput } from "./use-wizard-state";

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type CompanyProfile = components["schemas"]["CompanyProfile"];
type Proposal = components["schemas"]["OnboardingCompanyProposal"];

type ConfirmSubmission = Readonly<{
  values: CompanyForm;
  read: CompanySiteRead | null;
  proposal: Proposal | undefined;
  selectedFactKeys: string[];
  answers: readonly ClarifyAnswer[];
  onSubmitted: (proposalHash: string | undefined) => void;
}>;

// A finished read confirms against its staged draft; anything else saves the
// profile directly.
async function submitConfirm({
  values,
  read,
  proposal,
  selectedFactKeys,
  answers,
  onSubmitted,
}: ConfirmSubmission): Promise<CompanyProfile> {
  const profileInput = {
    ...values,
    display_name: values.display_name.trim(),
    offer_summary: values.offer_summary.trim(),
    icp: values.icp.trim(),
    legal_name: values.legal_name.trim(),
    registered_address: values.registered_address.trim(),
    register_vat: values.register_vat.trim(),
    industry: values.industry.trim(),
  };
  // When the proposal endpoint failed, the read snapshot carries the same
  // version pair, so the staged-confirm contract still holds.
  const proposalData =
    proposal ?? (read !== null ? proposalFromRead(read) : undefined);
  onSubmitted(proposalData?.proposal_hash);
  const result =
    read !== null &&
    (read.status === "ready" || read.status === "partial") &&
    proposalData?.draft_version !== undefined &&
    proposalData.proposal_hash !== undefined
      ? await api.POST("/company/site-reads/{readId}/confirm", {
          params: {
            path: { readId: read.id },
            header: { "Idempotency-Key": crypto.randomUUID() },
          },
          body: {
            draft_version: proposalData.draft_version,
            proposal_hash: proposalData.proposal_hash,
            profile: profileInput,
            selected_fact_keys: selectedFactKeys,
            resolutions: resolutionsFromAnswers(read.comparisons, answers),
          },
        })
      : await api.PUT("/company", { body: profileInput });
  const { data, error } = result;
  if (error) {
    throwProblem(error);
  }
  return data;
}

type CompanyConfirmArgs = Readonly<{
  dispatch: Dispatch<ConversationEvent>;
  persist: (input: WizardPersistInput) => Promise<boolean>;
  draftRef: Readonly<{ current: CompanyDraft }>;
  selectedFactKeys: string[];
  prevSnapshot: Readonly<{ current: CompanySiteRead | null }>;
  siteRead: UseQueryResult<CompanySiteRead, Error>;
  proposal: UseQueryResult<Proposal, Error>;
  answers: readonly ClarifyAnswer[];
  t: ReturnType<typeof useT>;
}>;

/**
 * The one explicit confirmation of the company act, and its recovery from
 * each refusal the server documents.
 */
export function useCompanyConfirm({
  dispatch,
  persist,
  draftRef,
  selectedFactKeys,
  prevSnapshot,
  siteRead,
  proposal,
  answers,
  t,
}: CompanyConfirmArgs) {
  const queryClient = useQueryClient();

  // The one route off the review, for this attempt's success and for an
  // earlier attempt that already confirmed.
  const finishConfirm = useCallback(
    (profileData: CompanyProfile) => {
      storeCompany(queryClient, profileData);
      // The checkpoint lets the classic coordinator resume at the right step
      // and role if the user switches shells.
      void persist({
        step: "basis",
        mode: prevSnapshot.current !== null ? "website" : "manual",
        readId: prevSnapshot.current?.id ?? null,
        values: draftRef.current.values,
        factKeys: selectedFactKeys,
      });
      dispatch({ type: "COMPANY_CONFIRMED" });
    },
    [dispatch, persist, prevSnapshot, queryClient, selectedFactKeys, draftRef],
  );

  const recovery = useConfirmRecovery({
    refetchRead: siteRead.refetch,
    refetchProposal: proposal.refetch,
    finishConfirm,
  });

  const confirm = useMutation({
    mutationFn: () =>
      submitConfirm({
        values: draftRef.current.values,
        read: prevSnapshot.current,
        proposal: proposal.data,
        selectedFactKeys,
        answers,
        onSubmitted: recovery.recordSubmitted,
      }),
    onMutate: recovery.resetBeforeAttempt,
    onSuccess: finishConfirm,
    onError: recovery.handleRefusal,
  });

  // The generic save-failed banner, only for a failure no notice already
  // explains. An `already_confirmed` shows none while its company loads, since
  // that save in fact went through.
  const bannerMessage =
    confirm.isError && recovery.notice === null && !recovery.awaitingCompanyLoad
      ? problemMessageOf(confirm.error, t)
      : null;
  const refusal: ConfirmRefusal | null =
    recovery.refusal === null
      ? null
      : {
          message: t(recovery.refusal.messageKey),
          retry: recovery.refusal.retry,
        };

  return {
    confirm,
    blocked: recovery.blocked,
    bannerMessage,
    refusal,
  };
}
