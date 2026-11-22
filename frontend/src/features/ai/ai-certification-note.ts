// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { useT } from "../../i18n";

type T = ReturnType<typeof useT>;
type Candidate = components["schemas"]["AiRouteCandidate"];

/**
 * What the certification records say about the model a tier binding serves for
 * one task. The sentence follows the record's verdict, so it never states a
 * failure the records no longer show. A model no record measured says so.
 */
export function certificationNote(candidate: Candidate, t: T): string {
  const model = candidate.model;
  const cert = candidate.certification;
  if (!cert) return t("aiAdmin.certification.unrecorded", { model });
  let head: string;
  switch (cert.verdict) {
    case "certified":
      head = t("aiAdmin.certification.certified", { model });
      break;
    case "supported_degraded":
      head = t("aiAdmin.certification.degraded", { model });
      break;
    case "not_supported":
      head = t("aiAdmin.certification.notSupported", { model });
      break;
    default:
      head = t("aiAdmin.certification.unrecorded", { model });
  }
  return cert.current ? head : `${head} ${t("aiAdmin.certification.stale")}`;
}
