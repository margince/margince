import type { Dispatch, SetStateAction } from "react";
import { useCallback } from "react";
import type { components } from "../../../api/schema";
import type { useT } from "../../../i18n";
import type { Locale } from "../../../i18n/locale";
import {
  useSetupDecline,
  useSetupDeclines,
} from "../../../lib/installation-setup.decline";
import { useInstallationSetup } from "../../../lib/installation-setup-query";
import {
  modelSkipped,
  outstandingStep,
} from "../../sign-in/installation-setup";
import type { CompanyDraft } from "../onboarding";
import { changeDraftField, normalizeUrl } from "../onboarding";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { gateNoticeFor } from "./gate-notice";
import type { useCompanyRead } from "./use-company-read";
import { safeStartError } from "./use-company-read";

type CompanySiteRead = components["schemas"]["CompanySiteRead"];

type CompanyGateArgs = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  setDraft: (update: SetStateAction<CompanyDraft>) => void;
  startRead: ReturnType<typeof useCompanyRead>["startRead"];
  read: CompanySiteRead | null;
  locale: Locale;
  t: ReturnType<typeof useT>;
}>;

/**
 * The company act's first face: the website question and the read theatre,
 * full-screen with no thread, panel or composer.
 *
 * It covers the whole span before there is anything to review and nothing
 * else. An in-flight start or an unarrived first snapshot is still waiting,
 * so it keeps the screen the reader is already on.
 */
export function useCompanyGate({
  state,
  dispatch,
  setDraft,
  startRead,
  read,
  locale,
  t,
}: CompanyGateArgs) {
  const setup = useInstallationSetup();
  const declined = useSetupDeclines();
  const { withdraw } = useSetupDecline();

  // The gate's field is the one place a website address is typed. It hands
  // back a bare host, and normalizeUrl is the one spelling of an address.
  const startFromGate = useCallback(
    (host: string) => {
      const norm = normalizeUrl(host);
      if (!norm.ok || startRead.isPending) {
        return;
      }
      setDraft((current) => changeDraftField(current, "website", norm.full));
      dispatch({ type: "URL_SUBMITTED", url: norm.full });
      startRead.mutate(norm.full);
    },
    [dispatch, setDraft, startRead],
  );

  const beforeReview =
    state.phase === "co.intro" || state.phase === "co.reading";
  const readOwned = state.phase === "co.reading" && state.activeReadId !== null;
  return {
    beforeReview,
    // An installation with no model bound cannot read a site, so setup comes
    // before the website question. `outstandingStep` answers only with a step
    // the setup screen has a panel for.
    setupOutstanding:
      beforeReview && outstandingStep(setup.data, declined) !== undefined,
    // The reader skipped the model: no website can be read, so the gate
    // offers the manual interview and a way back to the key.
    modelSkipped: modelSkipped(setup.data, declined),
    askForModel: () => withdraw("ai_models"),
    notice: gateNoticeFor({
      state,
      read,
      startError: startRead.isError ? safeStartError(startRead.error, t) : null,
      translate: t,
      failedWithDetail: (detail) => t("ob.gate.startFailed", { detail }),
      pausedWithDetail: (detail) => t("ob.gate.readPaused", { detail }),
    }),
    scan:
      readOwned && read
        ? { read, host: normalizeUrl(read.root_url).host, locale }
        : undefined,
    // A run whose first snapshot has not arrived keeps the Core working, so
    // the column does not change shape twice in half a second.
    running: startRead.isPending || (readOwned && !read),
    onSubmit: startFromGate,
  };
}
