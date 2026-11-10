import { type ComponentProps, type ReactNode, Suspense } from "react";
import {
  suggestedName,
  useDealSuggestion,
  useDealSuggestions,
} from "./components/dealsuggestions.queries";
import { QueryGate } from "./design-system/querystates";
import { lazy } from "./i18n/lazy";

const SuggestionCard = lazy(async () => ({
  default: (await import("./components/dealsuggestion")).DealSuggestionCard,
}));
function DealSuggestionCard(props: ComponentProps<typeof SuggestionCard>) {
  return (
    <Suspense fallback={null}>
      <SuggestionCard {...props} />
    </Suspense>
  );
}
const DealPicker = lazy(async () => ({
  default: (await import("./components/companydealpicker")).CompanyDealPicker,
}));
function CompanyDealPicker(props: ComponentProps<typeof DealPicker>) {
  return (
    <Suspense fallback={null}>
      <DealPicker {...props} />
    </Suspense>
  );
}

const LazyApprovalDetailModal = lazy(async () => ({
  default: (await import("./features/automations/approvaleditor"))
    .ApprovalDetailModal,
}));
function ApprovalDetailModal(
  props: ComponentProps<typeof LazyApprovalDetailModal>,
) {
  return (
    <Suspense fallback={null}>
      <LazyApprovalDetailModal {...props} />
    </Suspense>
  );
}
const LazyOpenActivityDrawer = lazy(async () => ({
  default: (await import("./features/voice-knowledge/activitydrawer"))
    .OpenActivityDrawer,
}));
function OpenActivityDrawer(
  props: ComponentProps<typeof LazyOpenActivityDrawer>,
) {
  return (
    <Suspense fallback={null}>
      <LazyOpenActivityDrawer {...props} />
    </Suspense>
  );
}

import { useT } from "./i18n";
import { type ReviewActions, ReviewActionsContext } from "./lib/reviewactions";

const NewDealAction = lazy(async () => ({
  default: (await import("./features/companies/companyactions")).NewDealAction,
}));
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
