// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { humanizeToken } from "./audit";

export type CriterionKind = components["schemas"]["StageCriterionKind"];

// Keyed by the generated union, so a kind added to crm.yaml fails the typecheck here.
export const CRITERION_KIND_LABEL: Readonly<Record<CriterionKind, MessageKey>> =
  {
    buyer_confirmed: "stage.criteria.kindBuyerConfirmed",
    event_held: "stage.criteria.kindEventHeld",
    document_signed: "stage.criteria.kindDocumentSigned",
    role_identified: "stage.criteria.kindRoleIdentified",
    terms_accepted: "stage.criteria.kindTermsAccepted",
    custom: "stage.criteria.kindCustom",
  };

// The report types a kind as a plain string, so a kind this build does not know still reads as words.
export function criterionKindLabel(kind: string, t: Translator): string {
  return isCriterionKind(kind)
    ? t(CRITERION_KIND_LABEL[kind])
    : humanizeToken(kind);
}

function isCriterionKind(kind: string): kind is CriterionKind {
  return Object.hasOwn(CRITERION_KIND_LABEL, kind);
}
