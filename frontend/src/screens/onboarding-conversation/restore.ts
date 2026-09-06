import type { components } from "../../api/schema";
import { formatNumber } from "../../format/format";
import type { Locale } from "../../i18n";
import type { NarrationEntry, ResumePoint } from "./conversation-types";

// Session restore as a pure decision: server state in, a start plan out.
// The wizard state's `path` field is THE member signal; an existing company
// profile is only the fallback when no state row exists (a returning creator
// has both a state row and a saved company, and must NOT be demoted to the
// member path, which would silently skip the installation acts). A member's
// company is settled before they arrive, so their plan always opens confirmed
// and lands on a personal act — the voice act first. Recap turns are derived
// here from server facts, never persisted narration, so a reload summarizes
// what happened instead of replaying it.

type OnboardingState = components["schemas"]["OnboardingState"];
type CompanyProfile = components["schemas"]["CompanyProfile"];
type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];
type CompanySiteRead = components["schemas"]["CompanySiteRead"];

export type VoiceRestoreProbe = Readonly<{
  /** An active or candidate version exists: the voice was actually built. */
  built: boolean;
  /** The server corpus meter; null when no voice profile exists yet. */
  summary: CorpusSummary | null;
}>;

export type RestoreInputs = Readonly<{
  /** GET /onboarding/state; null when nothing was persisted (404). */
  state: OnboardingState | null;
  /** GET /company; null while no human confirmed one (404). */
  profile: CompanyProfile | null;
  /** Voice server truth; null when the probe was not needed (member path,
   * or the journey has not reached the voice act). */
  voice: VoiceRestoreProbe | null;
  /** The read the persisted site_read_id points at (only fetched while the
   * company act is still open); null when none was persisted or it is gone. */
  read: CompanySiteRead | null;
  /** The OAuth return deep link (#/onboarding/connect/...) lands mid-journey
   * and must reopen the connect act, exactly like the classic coordinator. */
  routeConnect: boolean;
  /** The recap entries below are sentences a person reads, and the counts in
   * them are rendered here rather than at the renderer — a ThreadEntry's
   * params hold already-formatted strings. */
  locale: Locale;
}>;

export type RestorePlan =
  | { kind: "complete" }
  | {
      kind: "start";
      memberPath: boolean;
      companyConfirmed: boolean;
      /** Where RESUME lands; null means the company act is still open. */
      resumeTarget: ResumePoint | null;
      /** A persisted read worth reattaching: terminal ready/partial (its
       * review is one proposal fetch away) or still running (polling
       * resumes). The shell dispatches READ_STARTED for it so the finished
       * work is reachable again instead of stranded behind a reload. */
      adoptRead: CompanySiteRead | null;
      recap: readonly NarrationEntry[];
    };

// The read states a reload reattaches to. failed/deferred reopen fresh with
// an honest recap line instead (a failed run has nothing to review, and a
// deferred one restarts cleanly when the user re-submits); confirmed and
// abandoned are post-outcome lifecycle states with nothing left to do here.
const adoptableReadStates = new Set<CompanySiteRead["status"]>([
  "ready",
  "partial",
  "queued",
  "reading",
]);

function readRecap(read: CompanySiteRead, locale: Locale): NarrationEntry[] {
  const host = new URL(read.root_url).hostname;
  if (read.status === "ready" || read.status === "partial") {
    return [
      {
        kind: "narration",
        id: "recap:read-terminal",
        i18nKey: "ob.conv.recap.readTerminal",
        params: {
          host,
          count: formatNumber(read.profile_fields.length, locale),
        },
      },
    ];
  }
  if (read.status === "queued" || read.status === "reading") {
    return [
      {
        kind: "narration",
        id: "recap:read-reading",
        i18nKey: "ob.conv.recap.readReading",
        params: { host, pages: formatNumber(read.pages_read ?? 0, locale) },
      },
    ];
  }
  if (read.status === "failed") {
    return [
      {
        kind: "narration",
        id: "recap:read-failed",
        i18nKey: "ob.conv.recap.readFailed",
        params: { host },
      },
    ];
  }
  if (read.status === "deferred") {
    return [
      {
        kind: "narration",
        id: "recap:read-deferred",
        i18nKey: "ob.conv.recap.readDeferred",
        params: { host },
      },
    ];
  }
  return [];
}

// The wizard step values that mean the company confirmation already
// happened (the classic coordinator only advances past step "read"/"confirm"
// by persisting the confirmed company).
const confirmedSteps = new Set<OnboardingState["step"]>([
  "basis",
  "invite",
  "team",
  "voice",
  "results",
  "connect",
]);

// Where a confirmed journey resumes, creator and member alike: the creator-only
// steps land in their own acts, and the server never writes those against a
// member, so a member's row can only name a personal step.
function resumeTarget(state: OnboardingState): ResumePoint {
  // "results" is a row written before the invite existed, by a journey whose
  // voice act had already concluded: the recap it pointed at is gone, and
  // what came after the recap was connect.
  if (state.step === "connect" || state.step === "results") {
    return "cn.consent";
  }
  if (state.step === "basis") {
    return "bs.ask";
  }
  if (state.step === "invite") {
    return "in.ask";
  }
  if (state.step === "team") {
    return "tm.ask";
  }
  // step "voice": the act is still open, and lands straight on the collect
  // scene. A skip recorded early is honored; otherwise collection reopens
  // (or starts) with whatever corpus the server already holds.
  return state.voice_skipped ? "vo.skipped" : "vo.collecting";
}

function recapEntries(
  inputs: RestoreInputs,
  target: ResumePoint,
): NarrationEntry[] {
  const { state, profile, voice, locale } = inputs;
  const entries: NarrationEntry[] = [
    { kind: "narration", id: "recap:back", i18nKey: "ob.conv.recap.back" },
  ];
  // The company act's recap: confirmed with the saved name, or the honest
  // "not saved" when the state row claims progress the profile lacks.
  if (profile !== null) {
    entries.push({
      kind: "narration",
      id: "recap:company",
      i18nKey: "ob.conv.recap.company",
      params: { name: profile.display_name },
    });
  } else {
    entries.push({
      kind: "narration",
      id: "recap:company-unsaved",
      i18nKey: "ob.conv.recap.companyUnsaved",
    });
  }
  // The voice act's recap, only once that act concluded or holds material.
  if (voice?.built === true) {
    entries.push({
      kind: "narration",
      id: "recap:voice-built",
      i18nKey: "ob.conv.recap.voiceBuilt",
    });
  } else if (state?.voice_skipped === true) {
    entries.push({
      kind: "narration",
      id: "recap:voice-skipped",
      i18nKey: "ob.conv.recap.voiceSkipped",
    });
  } else if (target === "vo.collecting") {
    entries.push({
      kind: "narration",
      id: "recap:corpus",
      i18nKey: "ob.conv.recap.corpus",
      params: {
        words: formatNumber(voice?.summary?.total_words ?? 0, locale),
      },
    });
  }
  return entries;
}

// A member's company was confirmed before they arrived, so their plan always
// opens confirmed. A first visit has no row and nothing to recap: the journey
// opens at voice. A returning member resumes where their row says, and the
// OAuth return deep link reopens connect exactly as it does for a creator.
function memberPlan(
  inputs: RestoreInputs,
  state: OnboardingState | null,
): RestorePlan {
  let target: ResumePoint = "vo.collecting";
  if (inputs.routeConnect) {
    target = "cn.consent";
  } else if (state !== null) {
    target = resumeTarget(state);
  }
  return {
    kind: "start",
    memberPath: true,
    companyConfirmed: true,
    resumeTarget: target,
    adoptRead: null,
    recap: state === null ? [] : recapEntries(inputs, target),
  };
}

export function restorePlan(inputs: RestoreInputs): RestorePlan {
  const { state, profile, read, routeConnect, locale } = inputs;
  // "complete" is the wizard row's own word, and it can outrun the record it
  // claims: the state row and the company profile are separate writes, and the
  // connect act persists completion without requiring a saved profile. An
  // installation whose profile read comes back absent is not finished, whatever
  // the row says.
  //
  // Reporting it finished anyway does not merely show the wrong screen. The
  // app's onboarding gate sends every route back here while the profile is
  // absent, so a "complete" verdict answers by leaving — and the two rewrite
  // the hash at each other until React trips its nested-update limit and the
  // whole shell unmounts. The disagreement is already modelled a few lines
  // below as the "company unsaved" recap; this is the same fact, decided
  // earlier.
  if (state?.step === "complete" && profile !== null) {
    return { kind: "complete" };
  }
  const memberPath =
    state !== null ? state.path === "member" : profile !== null;
  if (memberPath) {
    return memberPlan(inputs, state);
  }
  const companyConfirmed = state !== null && confirmedSteps.has(state.step);
  if (state === null || !companyConfirmed) {
    // The company act is still open (fresh, or step read/confirm). A
    // persisted read reattaches so its progress or finished review is
    // reachable; a failed or deferred one reopens fresh with an honest line.
    return {
      kind: "start",
      memberPath,
      companyConfirmed: false,
      resumeTarget: null,
      adoptRead:
        read !== null && adoptableReadStates.has(read.status) ? read : null,
      recap: read !== null ? readRecap(read, locale) : [],
    };
  }
  const target: ResumePoint = routeConnect ? "cn.consent" : resumeTarget(state);
  return {
    kind: "start",
    memberPath: false,
    companyConfirmed: true,
    resumeTarget: target,
    adoptRead: null,
    recap: recapEntries(inputs, target),
  };
}
