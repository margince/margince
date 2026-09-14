// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record-360 kit's public surface. Deal360, Company360 and Contact360 all
// import from here; see README.md for what belongs in it.

export { BriefTitle } from "./brieftitle";
export {
  type BriefSentence,
  type CitationChip,
  Citations,
  type Cited,
  type CitedKind,
  type CitedSibling,
  citationChips,
  SentenceList,
  WrittenBy,
  type WrittenByWriter,
} from "./citations";
export {
  EvidenceSources,
  fromCitations,
  fromDealMove,
} from "./evidencesources";
export { incompleteGraph } from "./graphcompleteness";
export {
  dealRoleLabel,
  projectRoleLabel,
  signalKindLabel,
  signalTone,
} from "./labels";
export {
  isLate,
  MOMENT_EVIDENCE_LABEL,
  MOMENT_RULE_LABEL,
  MomentRow,
  momentGrounding,
  momentIsARow,
  standingTone,
} from "./moment";
export { CallCard, RecordReading, RecordReadingPair } from "./reading";
export {
  RecordSpine,
  type SpineCommercial,
  type SpineSource,
} from "./spine";
export { ThreadFailed } from "./threadfailed";
export { timelineSpineSource } from "./timelinespine";
export { TimelineThread } from "./timelinethread";
export { FoundMove, TodayPanel, TodoRow, WithheldNotice } from "./today";
export {
  type Grounding,
  Proof,
  type Signal,
  SignalStrip,
  type SignalTone,
  type StandingTone,
  VerdictHead,
} from "./verdict";
