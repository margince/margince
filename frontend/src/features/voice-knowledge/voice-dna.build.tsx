import { useMutation } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { SettingRow } from "../../design-system/settingrow";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { deferralDetail } from "../../lib/deferral-detail";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import type { VoiceProfile } from "./voice-dna";
import { useVoiceSources } from "./voice-samples";

// The mutation sink logs diagnostics; the card renders a safe explanation.
// Rejections are unknown because promises can reject with non-Error values.
export function reportFailure(
  setError: (message: string) => void,
  t: ReturnType<typeof useT>,
): (error: unknown) => void {
  return (error: unknown) => {
    setError(problemMessageOf(error, t));
  };
}

// The two ADR-0066 maturity thresholds, mirrored so the build control can say
// how far a corpus still has to go. The server decides what state a profile is
// in (VoiceProfile.maturity); these only phrase the distance to the next one.
export const VOICE_FIRST_BUILD_WORDS = 800;

const VOICE_FULL_BUILD_WORDS = 4000;

/** How a build ended, and the server's own words about it when it has any. */
type BuildOutcome = Readonly<{
  status: "succeeded" | "failed" | "deferred" | "pending";
  /** `status_detail` from the build row: safe operator guidance, already
   * written for a reader. Null when the server had nothing to add. */
  detail?: string | null;
}>;

// What the status line says at each point in a build's life: running, finished,
// or never started. It is a named function because the control around it is at
// the complexity ceiling. A reader asking what the line says should find one
// function that answers.
function buildStatusLine(
  t: ReturnType<typeof useT>,
  running: boolean,
  outcome: BuildOutcome | null,
): string {
  if (running) {
    return t("settings.voice.buildRunning");
  }
  if (outcome) {
    return buildOutcomeText(t, outcome);
  }
  return "";
}

// What the reader is told a finished build did.
//
// The server's own sentence wins whenever it sent one. Only the server knows
// the cause: a provider out of budget, a model answer it could not read, a
// missing configuration. The local strings are the fallback for an older
// server, and for the outcomes that carry no detail.
function buildOutcomeText(
  t: ReturnType<typeof useT>,
  outcome: BuildOutcome,
): string {
  const detail = outcome.detail?.trim();
  if (detail) {
    return detail;
  }
  return t(`settings.voice.buildStatus.${outcome.status}`);
}

// Build creates a durable background build; poll to a terminal state. A slow or
// budget-deferred build is reported as such, not spun on forever. The hero and
// the Builds card both start builds, so the one engine behind them lives here.
export function useVoiceBuild(profile: VoiceProfile, onBuilt: () => void) {
  const t = useT();
  const { locale } = useLocale();
  // The outcome and what the server said about it. A terminal build carries
  // `status_detail`, safe operator guidance the server wrote for this moment.
  // Without it a spending cap, an unreadable model answer and a broken
  // provider all read as one fixed sentence, and "try again" cannot help.
  const [outcome, setOutcome] = useState<BuildOutcome | null>(null);
  const [error, setError] = useState<string | null>(null);

  const build = useMutation({
    mutationFn: async (): Promise<BuildOutcome> => {
      const created = await api.POST("/voice-profiles/{id}/builds", {
        params: { path: { id: profile.id } },
        body: { reason: "manual" },
      });
      if (created.error) {
        throwProblem(created.error);
      }
      const buildId = created.data.id;
      for (let attempt = 0; attempt < 40; attempt++) {
        const data = unwrap(
          await api.GET("/voice-profiles/{id}/builds/{buildId}", {
            params: { path: { id: profile.id, buildId } },
          }),
        );
        // Three comparisons, not a set lookup: they prove to the compiler that
        // the status is one a BuildOutcome may hold. A new server state then
        // cannot pass through untyped.
        if (
          data.status === "succeeded" ||
          data.status === "failed" ||
          data.status === "deferred"
        ) {
          return { status: data.status, detail: deferralDetail(t, data) };
        }
        await new Promise((resolve) => {
          globalThis.setTimeout(resolve, 1500);
        });
      }
      // Still queued or running after the poll budget: "pending", not
      // "deferred" (which means the AI budget snoozed it). There is no detail
      // to carry, since nothing terminal happened.
      return { status: "pending", detail: null };
    },
    onSuccess: (finalOutcome) => {
      setOutcome(finalOutcome);
      setError(null);
      onBuilt();
    },
    onError: reportFailure(setError, t),
  });

  // The corpus summary rides the same query key CorpusManifest already read,
  // so asking for the word total here costs no extra request.
  const corpus = useVoiceSources(profile.id);
  // maturity is the server's verdict on whether a build can say anything. So
  // it decides whether the button is offered, not a locally recomputed
  // threshold. The word counts below only phrase the distance to the next state.
  const tooThin = profile.maturity === "collecting";
  // The distance is quoted only from a corpus total that loaded. A failed
  // fetch would read as zero words and announce "about 800 more words" when
  // the cause was the failure. The button's state follows maturity, which
  // comes from a different request.
  const words = corpus.isSuccess ? corpus.data.summary.total_words : null;
  const blocked =
    tooThin && words !== null
      ? t("settings.voice.buildNeedsWords", {
          n: formatNumber(Math.max(0, VOICE_FIRST_BUILD_WORDS - words), locale),
        })
      : null;
  const reach =
    profile.maturity === "provisional" && words !== null
      ? t("settings.voice.buildProvisional", {
          n: formatNumber(Math.max(0, VOICE_FULL_BUILD_WORDS - words), locale),
        })
      : null;

  return {
    build,
    error,
    tooThin,
    words,
    blocked,
    reach,
    progress: buildStatusLine(t, build.isPending, outcome),
  };
}

export type VoiceBuild = ReturnType<typeof useVoiceBuild>;

// The verb itself. "Rebuild" names a build that has happened. Before the first
// one there is nothing to redo, so the verb is the build.
export function BuildButton({
  run,
  profile,
  intakeBusy,
  variant = "ai",
  describedBy,
}: Readonly<{
  run: VoiceBuild;
  profile: VoiceProfile;
  intakeBusy: boolean;
  variant?: "ai" | "ghost";
  describedBy?: string;
}>) {
  const t = useT();
  return (
    <Button
      variant={variant}
      // A refusal the reader can act on, attached to the control rather than
      // left in a `title` no screen reader announces on a disabled button.
      reasonId={run.blocked === null ? undefined : describedBy}
      aria-describedby={describedBy}
      disabled={!run.build.isPending && (run.tooThin || intakeBusy)}
      pending={run.build.isPending}
      busyLabel={t("settings.voice.building")}
      onClick={() => run.build.mutate()}
    >
      <Sparkles aria-hidden />{" "}
      {t(
        (profile.profile_version ?? 0) > 0
          ? "settings.voice.rebuild"
          : "settings.voice.buildFirst",
      )}
    </Button>
  );
}

// One row: what the verb does on the left, the verb and the last outcome on the
// right. The distance still to go is the row's description, not a sentence by
// the button. It is two sentences, and only the naming column is wide enough.
export function BuildControls({
  profile,
  canEdit,
  onBuilt,
  intakeBusy = false,
}: Readonly<{
  canEdit: boolean;
  profile: VoiceProfile;
  onBuilt: () => void;
  intakeBusy?: boolean;
}>) {
  const t = useT();
  const run = useVoiceBuild(profile, onBuilt);
  return (
    <SettingRow
      label={t("settings.voice.buildRowLabel")}
      description={run.blocked ?? run.reach ?? undefined}
      control={(control) =>
        canEdit ? (
          <div className="vdna-buildcell">
            <BuildButton
              run={run}
              profile={profile}
              intakeBusy={intakeBusy}
              describedBy={control["aria-describedby"]}
            />
            {/* Mounted whether or not there is an outcome yet. A build runs for
                about a minute behind a poll, so its reader has looked away. A
                live region inserted with its text is not reliably announced,
                so the result they wait for would go unheard.

                While it runs this says so in words. The button's spinner and
                visually-hidden busy label serve a screen reader, not a reader
                looking at the page. A control that swallows a second press
                without saying why invites the press again. It also says the
                wait can be left: a build outlives this page. */}
            <p className="t-caption" role="status">
              {run.progress}
            </p>
            {run.error && <ErrorLine>{run.error}</ErrorLine>}
          </div>
        ) : null
      }
    />
  );
}
