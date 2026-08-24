// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record-360 kit's public surface. Deal360, Company360 and Person360 all
// import from here; see README.md for what belongs in it.

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
export { dealRoleLabel, signalKindLabel, signalTone } from "./labels";
export {
  incompleteGraph,
  OverlayFallback,
  RailPanel,
  SectionCard,
} from "./shells";
