import { type ComponentProps, lazy, type ReactNode, Suspense } from "react";
import { QueryGate } from "../design-system/querystates";
import { CompanyDealPicker } from "../features/deals/companydealpicker";
import { DealSuggestionCard } from "../features/deals/dealsuggestion";
import { useDealSuggestions } from "../features/deals/dealsuggestions.queries";
import { OpenActivityDrawer } from "../features/voice-knowledge/activitydrawer";
import { useT } from "../i18n";
import { type ReviewActions, ReviewActionsContext } from "../lib/reviewactions";

const NewDealAction = lazy(() =>
  import("../features/companies/companyactions").then((module) => ({
    default: module.NewDealAction,
  })),
);

function ComposedNewDealAction(props: ComponentProps<typeof NewDealAction>) {
  return (
    <Suspense fallback={null}>
      <NewDealAction {...props} />
    </Suspense>
  );
}

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

export const reviewActions: ReviewActions = {
  NewDealAction: ComposedNewDealAction,
  CompanyDealPicker,
  CoveredDealSuggestion,
  OpenActivityDrawer,
};

export function ReviewActionsProvider({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <ReviewActionsContext value={reviewActions}>
      {children}
    </ReviewActionsContext>
  );
}
