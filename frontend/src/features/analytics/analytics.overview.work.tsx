import { CardBoundary } from "../../design-system/cardboundary";
import { lazy } from "../../i18n/lazy";
export const MemberTable = lazy(
  async () => ({
    default: (await import("./analytics.overview.team.members")).MemberTable,
  }),
  "essential",
  CardBoundary,
);
export const UpcomingMeetings = lazy(
  async () => ({
    default: (await import("./analytics.overview.prospectingwork"))
      .UpcomingMeetings,
  }),
  "essential",
  CardBoundary,
);
export const HandoffCohort = lazy(
  async () => ({
    default: (await import("./analytics.overview.prospectingwork"))
      .HandoffCohort,
  }),
  "essential",
  CardBoundary,
);

export const AssessmentPanel = lazy(
  async () => ({
    default: (await import("./analytics.assessment")).AssessmentPanel,
  }),
  "essential",
  CardBoundary,
);

export const DecisionsPanel = lazy(
  async () => ({
    default: (await import("./analytics.decisions")).DecisionsPanel,
  }),
  "essential",
  CardBoundary,
);

export const ReviewDecisionsAction = lazy(
  async () => ({
    default: (await import("./analytics.decisions")).ReviewDecisionsAction,
  }),
  "essential",
  CardBoundary,
);

export const OutcomeWeeks = lazy(
  async () => ({
    default: (await import("./analytics.overview.prospectingweeks"))
      .OutcomeWeeks,
  }),
  "essential",
  CardBoundary,
);

export const OutcomeHistory = lazy(
  async () => ({
    default: (await import("./analytics.overview.prospectingweeks"))
      .OutcomeHistory,
  }),
  "essential",
  CardBoundary,
);
