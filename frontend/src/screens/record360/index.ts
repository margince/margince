// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record-360 kit's public surface. Deal360, Company360 and Person360 all
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
  dealRoleLabel,
  projectRoleLabel,
  signalKindLabel,
  signalTone,
} from "./labels";
export { CallCard, RecordReading, RecordReadingPair } from "./reading";
export {
  incompleteGraph,
  OverlayFallback,
  RailPanel,
  SectionCard,
} from "./shells";
export {
  RecordSpine,
  type SpineCommercial,
  type SpineSource,
} from "./spine";
export { timelineSpineSource } from "./timelinespine";
export { FoundMove, TodayPanel, TodoRow } from "./today";
export {
  type Grounding,
  Proof,
  type Signal,
  SignalStrip,
  type SignalTone,
  type StandingTone,
  VerdictHead,
} from "./verdict";
