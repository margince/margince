// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record-360 kit's public surface. Deal360, Company360 and Contact360 all
// import from here; see README.md for what belongs in it.

export {
  citationOpensRecord,
  openCitation,
} from "../../../components/citationroute";
export {
  type BriefSentence,
  type CitationChip,
  Citations,
  type Cited,
  type CitedKind,
  type CitedSibling,
  citationChips,
  type OpenReceipt,
  SentenceList,
  WrittenBy,
  type WrittenByWriter,
} from "../../../components/citations";
export {
  dealRoleLabel,
  projectRoleLabel,
  signalKindLabel,
  signalTone,
} from "../../../lib/recordlabels";
export { EvidenceSources, fromCitations } from "./evidencesources";
export { incompleteGraph } from "./graphcompleteness";
export { basisAddsARecord } from "./moment";
export { MomentEvidence } from "./momentevidence";
export { RecordReadingPair } from "./reading";
export { useSetAside } from "./setaside";
export { FoundMove, TodayPanel, TodoRow, WithheldNotice } from "./today";
export { type Signal, SignalStrip, type SignalTone } from "./verdict";
