import type { ReactNode } from "react";
import { QueryGate } from "../design-system/querystates";
import { NewDealAction } from "../features/companies/companyactions";
import { CompanyDealPicker } from "../features/deals/companydealpicker";
import { DealSuggestionCard } from "../features/deals/dealsuggestion";
import { useDealSuggestions } from "../features/deals/dealsuggestions.queries";
import { OpenActivityDrawer } from "../features/voice-knowledge/activitydrawer";
import { useT } from "../i18n";
import { type ReviewActions, ReviewActionsContext } from "../lib/reviewactions";

function CoveredDealSuggestion({
  companyId,
  suggestionId,
  open,
}: Readonly<{
  companyId: string | undefined;
  suggestionId: string;
  open: boolean;
}>) {
  const t = useT();
  const suggestions = useDealSuggestions(
    { company_id: companyId },
    open && companyId !== undefined,
  );
  return (
    <QueryGate query={suggestions} pendingLabel={t("stat.evidence")}>
      {(found) => {
        const suggestion = found.find((item) => item.id === suggestionId);
        return suggestion ? (
          <DealSuggestionCard suggestion={suggestion} />
        ) : (
          <p>{t("dealSuggestion.changed")}</p>
        );
      }}
    </QueryGate>
  );
}

const actions: ReviewActions = {
  NewDealAction,
  CompanyDealPicker,
  CoveredDealSuggestion,
  OpenActivityDrawer,
};

export function ReviewActionsProvider({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <ReviewActionsContext value={actions}>{children}</ReviewActionsContext>
  );
}
