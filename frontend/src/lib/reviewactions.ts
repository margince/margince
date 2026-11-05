import {
  type ComponentType,
  createContext,
  type ReactNode,
  useContext,
} from "react";
import type { components } from "../api/schema";

export type ReviewActions = Readonly<{
  DealSuggestionReader: ComponentType<{
    id: string;
    children: (state: {
      suggestion: components["schemas"]["DealSuggestion"] | undefined;
      label: string | undefined;
      error: unknown;
    }) => ReactNode;
  }>;
  DealSuggestionCard: ComponentType<{
    suggestion: components["schemas"]["DealSuggestion"];
  }>;
  ApprovalDetailModal: ComponentType<{
    approvalId: string;
    open: boolean;
    onClose: () => void;
  }>;

  NewDealAction: ComponentType<{
    companyId: string;
    companyName: string;
    onCreated: (deal: components["schemas"]["Deal"]) => void;
  }>;
  CompanyDealPicker: ComponentType<{
    companyId: string;
    value: string;
    onChange: (id: string) => void;
  }>;
  CoveredDealSuggestion: ComponentType<{
    companyId: string | undefined;
    suggestionId: string;
    open: boolean;
  }>;
  OpenActivityDrawer: ComponentType<{
    activityId: string | null;
    zone: string;
    onClose: () => void;
  }>;
}>;

export const ReviewActionsContext = createContext<ReviewActions | null>(null);

export function useReviewActions() {
  return useContext(ReviewActionsContext);
}
