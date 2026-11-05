import { type ComponentProps, lazy, type ReactNode, Suspense } from "react";
import { CompanyDealPicker } from "./components/companydealpicker";
import { DealSuggestionCard, suggestedName } from "./components/dealsuggestion";
import {
  useDealSuggestion,
  useDealSuggestions,
} from "./components/dealsuggestions.queries";
import { QueryGate } from "./design-system/querystates";
import { ApprovalDetailModal } from "./features/automations/approvaleditor";
import { OpenActivityDrawer } from "./features/voice-knowledge/activitydrawer";
import { useT } from "./i18n";
import { type ReviewActions, ReviewActionsContext } from "./lib/reviewactions";

const NewDealAction = lazy(() =>
  import("./features/companies/companyactions").then((module) => ({
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
  if (!open) return null;
  if (companyId === undefined) return <p>{t("dealSuggestion.changed")}</p>;
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

const DealSuggestionReader: ReviewActions["DealSuggestionReader"] = ({
  id,
  children,
}) => {
  const t = useT();
  const detail = useDealSuggestion(id);
  const suggestion = detail.error ? undefined : detail.data;
  return children({
    suggestion,
    label: suggestion ? suggestedName(suggestion, t) : undefined,
    error: detail.error,
  });
};

export const reviewActions: ReviewActions = {
  DealSuggestionReader,
  DealSuggestionCard,
  ApprovalDetailModal,
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
