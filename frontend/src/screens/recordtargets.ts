// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useRef } from "react";
import { api } from "../api/client";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { throwProblem } from "./common";

export const RELINK_KINDS = [
  "contact",
  "company",
  "deal",
  "lead",
  "project",
] as const;
export type RelinkKind = (typeof RELINK_KINDS)[number];

// Whether a cross-object search hit is something a message can be filed
// against. Asked as an ADMISSION rather than as a list of exclusions: the
// skip-list form named `activity` and `tag`, and silently admitted every type
// /search learned to return afterwards — which is how a product and an offer
// template became relink targets the moment the search enum widened. What this
// endpoint accepts is bounded and known; what search returns is not.
function isRelinkKind(type: string): type is RelinkKind {
  return (RELINK_KINDS as readonly string[]).includes(type);
}

// The relink target is chosen via cross-object search (/search covers every
// kind; the per-entity list endpoints don't all expose `q`). Each candidate's
// entity_type comes from its SearchResult.type, remembered here so the confirm
// can recover it — RecordPickerCandidate itself only carries {id,name}.
// Anything the relink enum does not name is dropped.
export function useRecordTargets() {
  const kindById = useRef(new Map<string, RelinkKind>());
  const search = useCallback(
    async (q: string): Promise<RecordPickerCandidate[]> => {
      const { data, error } = await api.GET("/search", {
        params: { query: { q, limit: 10 } },
      });
      if (error) throwProblem(error);
      const out: RecordPickerCandidate[] = [];
      for (const result of data.data) {
        // Only what a relink may point at. An activity is the message itself, a
        // tag is a word rather than something a message can be about, and a
        // catalog row is neither — none of them is a record this can be filed
        // against, and the endpoint's own enum is what says so.
        if (!result.type || !isRelinkKind(result.type)) continue;
        kindById.current.set(result.id, result.type);
        out.push({ id: result.id, name: result.title ?? result.id });
      }
      return out;
    },
    [],
  );
  return { search, kindOf: (id: string) => kindById.current.get(id) ?? null };
}
