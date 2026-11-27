import type { Query, QueryClient, QueryKey } from "@tanstack/react-query";
import type { components } from "../../../api/schema";
import { formatNumber } from "../../../format/format";
import type { Locale } from "../../../i18n";
import type { NarrationEntry, ResumePoint } from "./conversation-types";
import { SummaryOrder } from "./summary-order";

// Session restore as a pure decision: server state in, a start plan out.
// The wizard state's `path` field is THE member signal; a described
// installation is only the fallback when no state row exists (a returning creator
// has both a state row and a saved company, and must NOT be demoted to the
// member path, which would silently skip the installation acts). A member's
// company is settled before they arrive, so their plan always opens confirmed
// and lands on a personal act — the voice act first. Recap turns are derived
// here from server facts, never persisted narration, so a reload summarizes
// what happened instead of replaying it.

type OnboardingState = components["schemas"]["OnboardingState"];
type CompanyProfile = components["schemas"]["CompanyProfile"];
type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];
type CorpusSource = components["schemas"]["VoiceCorpusSource"];
type CompanySiteRead = components["schemas"]["CompanySiteRead"];

export type VoiceRestoreProbe = Readonly<{
  /** An active or candidate version exists: the voice was actually built. */
  built: boolean;
  /** The server corpus meter; null when no voice profile exists yet. */
  summary: CorpusSummary | null;
  /** The samples the meter counts, so the step lists them after a reload. */
  sources: readonly CorpusSource[];
  /** True once a removal landed and no read since has told the new count. */
  meterStale?: boolean;
}>;

/** The probe's query key; the voice step writes each corpus change into it,
 * so re-entering the step lists what the server holds now. */
export const VOICE_PROBE_KEY = ["onboarding-conv-voice"] as const;

export type ProbeChange = (
  old: VoiceRestoreProbe,
) => Partial<VoiceRestoreProbe>;

const emptyProbe: VoiceRestoreProbe = {
  built: false,
  summary: null,
  sources: [],
};

// The screen keeps the probe mounted, so an invalidation would read it again
// at once. Each change is written in instead, and a Back remount reads it.
// A probe the screen never read starts empty, so the step's corpus still
// outlives the step.
function patchVoiceProbe(client: QueryClient, change: ProbeChange) {
  client.setQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY, (old) => {
    const base = old ?? emptyProbe;
    return { ...base, ...change(base) };
  });
}

/** The one owner of the voice corpus: every voice step that opens over the
 *  same probe entry shares its write order, and writes through it. */
export type VoiceCorpusOwner = Readonly<{
  order: SummaryOrder;
  patch: (change: ProbeChange) => void;
}>;

type ProbeEntry = Query<VoiceRestoreProbe, Error, VoiceRestoreProbe, QueryKey>;

const owners = new WeakMap<ProbeEntry, VoiceCorpusOwner>();

// Keyed by the probe's cache entry, not the client. A seat change removes the
// entry, so a write the last seat left in flight lands in no corpus.
export function voiceCorpusOwner(client: QueryClient): VoiceCorpusOwner {
  const cache = client.getQueryCache();
  const probe = cache.build<
    VoiceRestoreProbe,
    Error,
    VoiceRestoreProbe,
    QueryKey
  >(client, { queryKey: VOICE_PROBE_KEY });
  const current = () =>
    cache.find<VoiceRestoreProbe>({ queryKey: VOICE_PROBE_KEY, exact: true });
  const known = owners.get(probe);
  if (known !== undefined) return known;
  const owner: VoiceCorpusOwner = {
    order: new SummaryOrder(),
    patch: (change) => {
      if (current() === probe) patchVoiceProbe(client, change);
    },
  };
  owners.set(probe, owner);
  return owner;
}

export type RestoreInputs = Readonly<{
  /** GET /onboarding/state; null when nothing was persisted (204). */
  state: OnboardingState | null;
  /** GET /company; null while no human confirmed one (204), and always null
   * for a seat that is not an admin, which may not read it. */
  profile: CompanyProfile | null;
  /** Whether the installation has described itself (useInstallationDescribed):
   * the saved profile for an admin, implied for any other seat. */
  described: boolean;
  /** Whether this seat holds the admin role, the only one that may save the
   * company: any other seat walks the member path, whatever its row says. */
  mayDescribe: boolean;
  /** Whether this seat may create a voice profile, which the first sample
   * mints: a seat without it never reaches the voice act. */
  mayTrainVoice: boolean;
  /** Voice server truth; null when the probe was not needed (member path,
   * or the journey has not reached the voice act). */
  voice: VoiceRestoreProbe | null;
  /** The read the persisted site_read_id points at (only fetched while the
   * company act is still open); null when none was persisted or it is gone. */
  read: CompanySiteRead | null;
  /** The connect step's own address, and the OAuth return under it
   * (#/onboarding/connect/...), reopen the connect act on a reload. */
  routeConnect: boolean;
  /** The recap entries below are sentences a reader reads, and the counts in
   * them are rendered here rather than at the renderer — a ThreadEntry's
   * params hold already-formatted strings. */
  locale: Locale;
}>;

export type RestorePlan =
  | { kind: "complete" }
  | {
      kind: "start";
      memberPath: boolean;
      /** Whether the voice act is offered (ConversationState.voiceStep). */
      voiceStep: boolean;
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
  // step "voice": the collect scene with the corpus the server holds, unless
  // a skip recorded here already led on to connect.
  return state.voice_skipped ? "cn.consent" : "vo.collecting";
}

function recapEntries(
  inputs: RestoreInputs,
  target: ResumePoint,
): NarrationEntry[] {
  const { state, profile, described, voice, locale } = inputs;
  const entries: NarrationEntry[] = [
    { kind: "narration", id: "recap:back", i18nKey: "ob.conv.recap.back" },
  ];
  // The company act's recap: confirmed with the saved name, or the honest
  // "not saved" when the state row claims progress the profile lacks. A seat
  // that may not read the name gets no company line: its company is settled.
  if (profile !== null) {
    entries.push({
      kind: "narration",
      id: "recap:company",
      i18nKey: "ob.conv.recap.company",
      params: { name: profile.display_name },
    });
  } else if (!described) {
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

// Where a confirmed journey lands. Its own connect address wins, and so does
// connect over a voice act the seat may not train.
function landingPoint(inputs: RestoreInputs, wanted: ResumePoint): ResumePoint {
  const pastVoice = !inputs.mayTrainVoice && wanted === "vo.collecting";
  return inputs.routeConnect || pastVoice ? "cn.consent" : wanted;
}

// A member's plan opens confirmed at voice, and a returning member resumes
// where their row says.
function memberPlan(
  inputs: RestoreInputs,
  state: OnboardingState | null,
): RestorePlan {
  const target = landingPoint(
    inputs,
    state === null ? "vo.collecting" : resumeTarget(state),
  );
  return {
    kind: "start",
    memberPath: true,
    voiceStep: inputs.mayTrainVoice,
    companyConfirmed: true,
    resumeTarget: target,
    adoptRead: null,
    recap: state === null ? [] : recapEntries(inputs, target),
  };
}

export function restorePlan(inputs: RestoreInputs): RestorePlan {
  const { state, described, mayDescribe, read, locale } = inputs;
  // "complete" is the wizard row's own word, and it can outrun the record it
  // claims: the state row and the company profile are separate writes, and the
  // connect act persists completion without requiring a saved profile. An
  // installation that is not described is not finished, whatever the row says.
  //
  // Reporting it finished anyway does not merely show the wrong screen. The
  // app's onboarding gate sends every route back here while the profile is
  // absent, so a "complete" verdict answers by leaving — and the two rewrite
  // the hash at each other until React trips its nested-update limit and the
  // whole shell unmounts. The disagreement is already modelled a few lines
  // below as the "company unsaved" recap; this is the same fact, decided
  // earlier.
  if (state?.step === "complete" && described) {
    return { kind: "complete" };
  }
  // A creator row outlives a demoted admin, and its company act ends in a save
  // the seat may no longer make.
  const memberPath =
    !mayDescribe || (state !== null ? state.path === "member" : described);
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
      voiceStep: inputs.mayTrainVoice,
      companyConfirmed: false,
      resumeTarget: null,
      adoptRead:
        read !== null && adoptableReadStates.has(read.status) ? read : null,
      recap: read !== null ? readRecap(read, locale) : [],
    };
  }
  const target = landingPoint(inputs, resumeTarget(state));
  return {
    kind: "start",
    memberPath: false,
    voiceStep: inputs.mayTrainVoice,
    companyConfirmed: true,
    resumeTarget: target,
    adoptRead: null,
    recap: recapEntries(inputs, target),
  };
}
