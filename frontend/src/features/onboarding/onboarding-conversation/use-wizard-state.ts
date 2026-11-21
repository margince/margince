import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useRef } from "react";
import type { components } from "../../../api/schema";
import { useAddressWriters } from "../../../lib/urlstate";
import type { CompanyForm } from "../onboarding";
import { wizardStateBody, writeWizardState } from "../onboarding";
import { onboardingProgressOptions } from "../onboarding-progress";
import { isCompanyField } from "./company-proposal";

// Minimal wizard-state persistence for the conversational shell: the server
// resolves GET /onboarding/company/proposal through the persisted
// site_read_id, and act checkpoints (company confirmed, finish) keep the
// classic coordinator able to take over mid-journey. Full restore is Phase
// 5. Writes are queued so a fast retry never races an earlier PUT, and a
// prior session's checkpoint is merged, never clobbered: blanks fill from
// the saved draft, and unspecified flags keep their saved values.

type OnboardingState = components["schemas"]["OnboardingState"];

type SavedState = {
  version: number;
  sourceMode: "website" | "manual" | null;
  siteReadId: string | null;
  websiteUrl: string | null;
  factKeys: string[];
  voiceSkipped: boolean;
  connectSkipped: boolean;
  draft: OnboardingState["company_draft"] | null;
  /** The server's row once the journey is complete, which no write changes. */
  finished: OnboardingState | null;
};

export type WizardPersistInput = Readonly<{
  /** The step the journey stands on once this write lands. */
  step: OnboardingState["step"];
  values: CompanyForm;
  /** Omitted fields inherit the previously saved state. */
  mode?: "website" | "manual" | null;
  readId?: string | null;
  factKeys?: string[];
  voiceSkipped?: boolean;
  connectSkipped?: boolean;
}>;

const FRESH_STATE: SavedState = {
  version: 0,
  sourceMode: null,
  siteReadId: null,
  websiteUrl: null,
  factKeys: [],
  voiceSkipped: false,
  connectSkipped: false,
  draft: null,
  finished: null,
};

function fromServerState(data: OnboardingState | null): SavedState {
  // Nothing persisted yet: the first write starts at version 0.
  if (data === null) {
    return FRESH_STATE;
  }
  return {
    version: typeof data.version === "number" ? data.version : 0,
    sourceMode: data.source_mode ?? null,
    siteReadId: data.site_read_id ?? null,
    websiteUrl: data.website_url ?? null,
    factKeys: Array.isArray(data.selected_fact_keys)
      ? data.selected_fact_keys
      : [],
    voiceSkipped: Boolean(data.voice_skipped),
    connectSkipped: Boolean(data.connect_skipped),
    draft: data.company_draft ?? null,
    finished: data.step === "complete" ? data : null,
  };
}

// A blank conversational field never clobbers a value an earlier session
// already saved; a non-blank current value always wins.
function mergedValues(
  values: CompanyForm,
  savedDraft: SavedState["draft"],
): CompanyForm {
  if (!savedDraft) {
    return values;
  }
  const merged = { ...values };
  for (const [key, savedValue] of Object.entries(savedDraft)) {
    if (
      typeof savedValue === "string" &&
      savedValue !== "" &&
      isCompanyField(key, merged) &&
      merged[key].trim() === ""
    ) {
      merged[key] = savedValue;
    }
  }
  return merged;
}

// One write on top of the saved base: current values (blanks filled from
// the saved draft), explicit input fields winning over saved ones. The
// server's answer is returned whole so the caller can publish it.
async function writeMerged(
  base: SavedState,
  input: WizardPersistInput,
): Promise<OnboardingState> {
  const values = mergedValues(input.values, base.draft);
  const website =
    values.website.trim() !== "" ? values.website : (base.websiteUrl ?? "");
  return writeWizardState(
    wizardStateBody({
      expectedVersion: base.version,
      step: input.step,
      mode: input.mode === undefined ? base.sourceMode : input.mode,
      readID: input.readId === undefined ? base.siteReadId : input.readId,
      norm: { ok: website.trim() !== "", full: website },
      values,
      factKeys: input.factKeys ?? base.factKeys,
      skippedVoice: input.voiceSkipped ?? base.voiceSkipped,
      skippedConnect: input.connectSkipped ?? base.connectSkipped,
    }),
  );
}

export function useWizardStatePersist() {
  const saved = useRef<SavedState | null>(null);
  const queue = useRef<Promise<boolean>>(Promise.resolve(true));
  const client = useQueryClient();
  const { navigateReplacing } = useAddressWriters();

  const persist = useCallback(
    (input: WizardPersistInput): Promise<boolean> => {
      queue.current = queue.current.then(async () => {
        try {
          saved.current ??= fromServerState(
            await client.fetchQuery({
              ...onboardingProgressOptions,
              staleTime: Number.POSITIVE_INFINITY,
            }),
          );
          let written: OnboardingState;
          try {
            written =
              saved.current.finished ??
              (await writeMerged(saved.current, input));
          } catch {
            // A refused write reads the row again: another tab may have
            // finished the journey, and the next write needs its version.
            const fresh = await client.fetchQuery({
              ...onboardingProgressOptions,
              staleTime: 0,
            });
            saved.current = fromServerState(fresh);
            if (fresh?.step !== "complete") {
              return false;
            }
            written = fresh;
          }
          saved.current = fromServerState(written);
          // The app shell gates on this same entry: a completion it never
          // learned of would send the finished journey straight back here.
          client.setQueryData(onboardingProgressOptions.queryKey, written);
          if (written.step === "complete" && input.step !== "complete") {
            // Another tab finished the journey, so this one goes Home too.
            navigateReplacing({ screen: "home" });
            return false;
          }
          return true;
        } catch {
          // Best-effort by design: the act must never stall on state
          // persistence. The caller learns via the false result (the proposal
          // join is gated on it and falls back to the site-read snapshot).
          // Invalidating forces a fresh version sync on the next attempt.
          saved.current = null;
          void client.invalidateQueries({
            queryKey: onboardingProgressOptions.queryKey,
            refetchType: "none",
          });
          return false;
        }
      });
      return queue.current;
    },
    [client, navigateReplacing],
  );

  return { persist };
}
