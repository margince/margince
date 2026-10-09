import type { SetStateAction } from "react";
import { useCallback, useRef, useState } from "react";
import type { components } from "../../api/schema";
import type { CompanyDraft, CompanyFieldName } from "../onboarding";
import { changeDraftField, EMPTY_DRAFT, formFromProfile } from "../onboarding";
import { draftWithLegalEntity } from "./company-proposal";
import type { SuggestedCompanyChange } from "./use-clarify-answers";

type CompanyProfile = components["schemas"]["CompanyProfile"];
type LegalEntity = components["schemas"]["CompanySiteReadLegalEntity"];

function initialDraft(profile: CompanyProfile | null): CompanyDraft {
  return profile
    ? { values: formFromProfile(profile), grounded: {}, edited: new Set() }
    : EMPTY_DRAFT;
}

/**
 * The company act's draft, seeded from the member path's existing company so a
 * confirmation never erases stored fields the read did not rediscover.
 *
 * Values, grounding and human-edit marks move together, as in the classic
 * coordinator, and the ref keeps callbacks reading the current draft.
 */
export function useCompanyDraft(profile: CompanyProfile | null) {
  const [draft, setDraftState] = useState<CompanyDraft>(() =>
    initialDraft(profile),
  );
  const draftRef = useRef<CompanyDraft>(draft);
  const setDraft = useCallback((update: SetStateAction<CompanyDraft>) => {
    const next =
      typeof update === "function" ? update(draftRef.current) : update;
    draftRef.current = next;
    setDraftState(next);
  }, []);

  const applyChanges = useCallback(
    (changes: readonly SuggestedCompanyChange[]) => {
      setDraft((current) => {
        let next = current;
        for (const change of changes) {
          next = changeDraftField(next, change.field, change.value);
        }
        return next;
      });
    },
    [setDraft],
  );

  const setField = (field: CompanyFieldName, value: string) =>
    setDraft((current) => changeDraftField(current, field, value));
  // A whole-entity pick keeps its provenance and its never-overwrite-an-edit
  // guard in the draft helper, which the dossier's entity cards call too.
  const pickEntity = (entity: LegalEntity) =>
    setDraft((current) => draftWithLegalEntity(current, entity));

  return { draft, draftRef, setDraft, applyChanges, setField, pickEntity };
}
