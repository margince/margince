import { lazy } from "../../i18n/lazy";
export const MemberTable = lazy(
  async () => ({
    default: (await import("./analytics.overview.team.members")).MemberTable,
  }),
  "essential",
);
export const UpcomingMeetings = lazy(
  async () => ({
    default: (await import("./analytics.overview.prospectingwork"))
      .UpcomingMeetings,
  }),
  "essential",
);
export const HandoffCohort = lazy(
  async () => ({
    default: (await import("./analytics.overview.prospectingwork"))
      .HandoffCohort,
  }),
  "essential",
);

export const AssessmentPanel = lazy(
  async () => ({
    default: (await import("./analytics.assessment")).AssessmentPanel,
  }),
  "essential",
);

export const DecisionsPanel = lazy(
  async () => ({
    default: (await import("./analytics.decisions")).DecisionsPanel,
  }),
  "essential",
);

export const ReviewDecisionsAction = lazy(
  async () => ({
    default: (await import("./analytics.decisions")).ReviewDecisionsAction,
  }),
  "essential",
);

export const OutcomeWeeks = lazy(
  async () => ({
    default: (await import("./analytics.overview.prospectingweeks"))
      .OutcomeWeeks,
  }),
  "essential",
);

export const OutcomeHistory = lazy(
  async () => ({
    default: (await import("./analytics.overview.prospectingweeks"))
      .OutcomeHistory,
  }),
  "essential",
);
