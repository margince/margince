import { useMutation, useQuery } from "@tanstack/react-query";
import { RotateCcw } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button, Card, Disclosure } from "../design-system/atoms";
import { formatDate, formatNumber, identifierNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { parseVoiceInsights, VoiceInsights } from "./voice-insights";
import "./voice-dna.css";
import { SurfaceState } from "../design-system/surfacestate";

type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];
type VoiceProfileDelta = components["schemas"]["VoiceProfileDelta"];
type VoiceLearningSummary = components["schemas"]["VoiceLearningSummary"];

type VersionsPage = { items: VoiceProfileVersion[]; next: string | null };
type DeltasPage = { items: VoiceProfileDelta[]; next: string | null };

// mergeById accumulates keyset pages without duplicating rows when a page
// is refetched after invalidation.
function mergeById<T extends { id: string }>(prev: T[], page: T[]): T[] {
  const seen = new Set(page.map((item) => item.id));
  return [...prev.filter((item) => !seen.has(item.id)), ...page];
}

export function useVoiceVersions(profileId: string | undefined) {
  return useQuery({
    queryKey: ["voice-versions", profileId],
    enabled: Boolean(profileId),
    queryFn: async (): Promise<VoiceProfileVersion[]> => {
      const { data, error } = await api.GET("/voice-profiles/{id}/versions", {
        params: { path: { id: profileId as string } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

// ActiveVoiceInsights renders the impress surface from the active version
// and, when a candidate awaits review, the apply/reject banner above it.
export function ActiveVoiceInsights({
  profileId,
  canEdit,
  onChanged,
}: Readonly<{
  profileId: string;
  canEdit: boolean;
  onChanged: () => void;
}>) {
  const t = useT();
  const versions = useVoiceVersions(profileId);
  return (
    <QueryGate query={versions} pendingLabel={t("settings.voice.title")}>
      {(list) => {
        const active = list.find((version) => version.status === "active");
        const candidate = list.find(
          (version) => version.status === "candidate",
        );
        return (
          <div>
            {candidate && (
              <CandidateBanner
                profileId={profileId}
                candidate={candidate}
                canEdit={canEdit}
                onChanged={onChanged}
              />
            )}
            {active && (
              <VoiceInsights
                data={parseVoiceInsights(active)}
                profileVersion={active.profile_version}
              />
            )}
          </div>
        );
      }}
    </QueryGate>
  );
}

// The evaluator writes its reasons for an operator reading a log, and they
// reached the owner verbatim: "median voice score 0.56 is below the 0.60 floor"
// says nothing to the person being asked to decide. Each known shape is stated
// again in words about THEIR voice and what to do about it.
//
// An unrecognized reason is shown as it came rather than dropped: a reason
// nobody can read is bad, and a reason nobody can SEE is worse — it would let
// a candidate be held back for a cause the owner is never told.
function reviewReasonText(
  t: ReturnType<typeof useT>,
  locale: Locale,
  reason: string,
): string {
  const lowScore =
    /median voice score ([\d.]+) is below the ([\d.]+) floor/.exec(reason);
  if (lowScore) {
    // Through the formatter, like every other figure on this page: the
    // evaluator writes "0.56" because it writes for a log, and a German reader
    // reads 0,56. A score handed straight to the sentence kept the log's
    // notation in front of a reader who does not use it.
    return t("voice.candidate.reason.lowScore", {
      score: formatNumber(Number(lowScore[1] ?? 0), locale),
      floor: formatNumber(Number(lowScore[2] ?? 0), locale),
    });
  }
  const hardFailures = /^(\d+) anti-AI hard failures survived/.exec(reason);
  if (hardFailures) {
    return t("voice.candidate.reason.hardFailures", {
      n: formatNumber(Number(hardFailures[1] ?? 0), locale),
    });
  }
  const rulesRemoved =
    /^(\d+) avoid and (\d+) register rules were removed/.exec(reason);
  if (rulesRemoved) {
    const dropped = Number(rulesRemoved[1] ?? 0) + Number(rulesRemoved[2] ?? 0);
    return t("voice.candidate.reason.rulesRemoved", {
      n: formatNumber(dropped, locale),
    });
  }
  if (reason.includes("malformed drafts during evaluation")) {
    return t("voice.candidate.reason.malformed");
  }
  return reason;
}

// A candidate never replaces the active voice silently: the owner applies
// or rejects it, with the version itself and the evaluator's reasons in view.
function CandidateBanner({
  profileId,
  candidate,
  canEdit,
  onChanged,
}: Readonly<{
  profileId: string;
  candidate: VoiceProfileVersion;
  canEdit: boolean;
  onChanged: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [error, setError] = useState<string | null>(null);
  const transition = useMutation({
    mutationFn: async (action: "apply" | "reject") => {
      const path =
        action === "apply"
          ? ("/voice-profiles/{id}/versions/{profileVersion}/apply" as const)
          : ("/voice-profiles/{id}/versions/{profileVersion}/reject" as const);
      const { error: err } = await api.POST(path, {
        params: {
          path: { id: profileId, profileVersion: candidate.profile_version },
          header: { "If-Match": String(candidate.version) },
        },
      });
      if (err) {
        throwProblem(err);
      }
    },
    onSuccess: () => {
      setError(null);
      onChanged();
    },
    onError: (e: Error) => setError(problemMessageOf(e, t)),
  });
  return (
    // The design system's own inset card, not a hand-rolled `<div className=
    // "card">`: a copy of the card surface is a second card the moment one of
    // its five chrome values moves, and the README forbids it by name.
    <Card as="div" inset className="vdna-candidate">
      <b>
        {t("voice.candidate.title", {
          n: identifierNumber(candidate.profile_version),
        })}
      </b>
      <p className="t-caption">{t("voice.candidate.whatItIs")}</p>
      {/* The decision this card asks for cannot be taken without the thing it
          is about. It used to show a title, the evaluator's raw sentences and
          two buttons — so "Use this version" meant approving writing the
          reader had never seen, and the only way to read it was to apply it
          first. The version is already in hand here; the same insights view
          the active voice uses renders it. */}
      <Disclosure summary={t("voice.candidate.reviewLabel")} open>
        <VoiceInsights
          data={parseVoiceInsights(candidate)}
          profileVersion={candidate.profile_version}
        />
      </Disclosure>
      {candidate.review_reasons.length > 0 && (
        <>
          <p className="t-caption vdna-label">
            {t("voice.candidate.concernsLabel")}
          </p>
          <ul className="vdna-reasons">
            {candidate.review_reasons.map((reason) => (
              <li key={reason}>{reviewReasonText(t, locale, reason)}</li>
            ))}
          </ul>
        </>
      )}
      <p className="t-caption">{t("voice.candidate.applyHint")}</p>
      {error && (
        <p className="t-caption" role="alert">
          {error}
        </p>
      )}
      {canEdit && (
        <div className="vdna-candidate-acts">
          <Button
            variant="primary"
            small
            disabled={transition.isPending}
            onClick={() => transition.mutate("apply")}
          >
            {t("voice.candidate.apply")}
          </Button>
          <Button
            small
            disabled={transition.isPending}
            onClick={() => transition.mutate("reject")}
          >
            {t("voice.candidate.reject")}
          </Button>
        </div>
      )}
    </Card>
  );
}

// VoiceHistory is the append-only record: every version with its rollback, and
// the learning-signal counters underneath them. It draws no heading of its own
// — the settings row that holds it names it, and a list that also titled itself
// would say it twice.
//
// The "what changed" delta timeline is its own component below: it is the
// diagnostic half of the same record, and the card keeps it behind a
// Disclosure rather than in front of every reader.
export function VoiceHistory({
  profileId,
  canEdit,
  onChanged,
}: Readonly<{
  profileId: string;
  canEdit: boolean;
  onChanged: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [versionCursor, setVersionCursor] = useState<string | undefined>();
  const [allVersions, setAllVersions] = useState<VoiceProfileVersion[]>([]);
  const versions = useQuery({
    queryKey: ["voice-versions", profileId, versionCursor ?? ""],
    queryFn: async (): Promise<VersionsPage> => {
      const { data, error } = await api.GET("/voice-profiles/{id}/versions", {
        params: {
          path: { id: profileId },
          query: versionCursor ? { cursor: versionCursor } : {},
        },
      });
      if (error) {
        throwProblem(error);
      }
      setAllVersions((prev) => mergeById(prev, data.data));
      return { items: data.data, next: data.page.next_cursor ?? null };
    },
  });
  const learning = useQuery({
    queryKey: ["voice-learning", profileId],
    queryFn: async (): Promise<VoiceLearningSummary> => {
      const { data, error } = await api.GET("/voice-profiles/{id}/learning", {
        params: { path: { id: profileId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  return (
    // `settingrow-measure` is the row primitive's hook for a control that takes
    // a stacked row's full width, which is the only place this list is drawn.
    <div className="settingrow-measure">
      <QueryGate query={versions} pendingLabel={t("voice.history.label")}>
        {(page) =>
          allVersions.length === 0 ? (
            <SurfaceState
              state="empty"
              emptyLabel={t("voice.history.empty")}
              loadingLabel={t("voice.history.label")}
            >
              {null}
            </SurfaceState>
          ) : (
            <div>
              <ul className="vdna-list">
                {[...allVersions]
                  .sort((a, b) => b.profile_version - a.profile_version)
                  .map((version) => (
                    <VersionRow
                      key={version.id}
                      profileId={profileId}
                      version={version}
                      canEdit={canEdit}
                      onChanged={onChanged}
                    />
                  ))}
              </ul>
              {page.next && (
                <Button
                  small
                  onClick={() => setVersionCursor(page.next ?? undefined)}
                >
                  {t("voice.history.loadMore")}
                </Button>
              )}
            </div>
          )
        }
      </QueryGate>
      <QueryGate query={learning} pendingLabel={t("voice.history.label")}>
        {(summary) => (
          <p className="t-caption vdna-learning">
            {t("voice.history.learning", {
              drafted: formatNumber(summary.drafted, locale),
              edited: formatNumber(summary.edited_sent, locale),
              rejected: formatNumber(summary.rejected, locale),
            })}
          </p>
        )}
      </QueryGate>
    </div>
  );
}

// VoiceChangeLog is what each build CHANGED, version to version: the
// classification the evaluator gave it and how it was activated. Diagnostic
// rather than actionable — nothing here is a control — which is why the card
// keeps it behind a Disclosure. It draws no heading either: the summary the
// reader opened is the heading.
//
// Its own component, and its own keyset cursor, so the version list beside it
// pages independently: one list running out of pages must not stop the other.
export function VoiceChangeLog({ profileId }: Readonly<{ profileId: string }>) {
  const t = useT();
  const [deltaCursor, setDeltaCursor] = useState<string | undefined>();
  const [allDeltas, setAllDeltas] = useState<VoiceProfileDelta[]>([]);
  const deltas = useQuery({
    queryKey: ["voice-deltas", profileId, deltaCursor ?? ""],
    queryFn: async (): Promise<DeltasPage> => {
      const { data, error } = await api.GET("/voice-profiles/{id}/deltas", {
        params: {
          path: { id: profileId },
          query: deltaCursor ? { cursor: deltaCursor } : {},
        },
      });
      if (error) {
        throwProblem(error);
      }
      setAllDeltas((prev) => mergeById(prev, data.data));
      return { items: data.data, next: data.page.next_cursor ?? null };
    },
  });
  return (
    <QueryGate query={deltas} pendingLabel={t("voice.history.label")}>
      {(page) =>
        allDeltas.length === 0 ? (
          <p className="t-caption">{t("voice.history.deltasEmpty")}</p>
        ) : (
          <div>
            <ul className="vdna-list">
              {[...allDeltas]
                .sort((a, b) => b.to_version - a.to_version)
                .map((delta) => (
                  <li key={delta.id} className="vdna-row">
                    <span>
                      {t("voice.history.deltaRow", {
                        from: identifierNumber(delta.from_version ?? 0),
                        to: identifierNumber(delta.to_version),
                      })}
                      {" · "}
                      {classificationLabel(t, delta.classification)} ·{" "}
                      {outcomeLabel(t, delta.activation_outcome)}
                    </span>
                  </li>
                ))}
            </ul>
            {page.next && (
              <Button
                small
                onClick={() => setDeltaCursor(page.next ?? undefined)}
              >
                {t("voice.history.loadMore")}
              </Button>
            )}
          </div>
        )
      }
    </QueryGate>
  );
}

function VersionRow({
  profileId,
  version,
  canEdit,
  onChanged,
}: Readonly<{
  profileId: string;
  version: VoiceProfileVersion;
  canEdit: boolean;
  onChanged: () => void;
}>) {
  const t = useT();
  const [error, setError] = useState<string | null>(null);
  const rollback = useMutation({
    mutationFn: async () => {
      const { error: err } = await api.POST(
        "/voice-profiles/{id}/versions/{profileVersion}/rollback",
        {
          params: {
            path: { id: profileId, profileVersion: version.profile_version },
          },
        },
      );
      if (err) {
        throwProblem(err);
      }
    },
    onSuccess: () => {
      setError(null);
      onChanged();
    },
    onError: (e: Error) => setError(problemMessageOf(e, t)),
  });
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <li className="vdna-row">
      <span>
        {t("voice.history.versionRow", {
          n: identifierNumber(version.profile_version),
        })}{" "}
        <Badge>{versionStatusLabel(t, version.status)}</Badge>
        {" · "}
        {/* `locale` is the app's own code ("en") — a valid BCP-47 tag, but a
            language-only one, and an unspecified region is exactly the gap
            A100 exists to close. Handed straight to Intl it resolves to en-US
            defaults and prints 8/21/2026 where every other date in this
            product prints 21/08/2026. The regioned tag comes from format/, and
            the record's zone with it: a profile version is stamped by the
            record, not by where its reader sits. */}
        {formatDate(version.created_at, locale, recordZone)}
      </span>
      {canEdit && version.status === "superseded" && (
        <button
          type="button"
          className="iconbtn vdna-row-verb"
          aria-label={t("voice.history.rollback", {
            n: identifierNumber(version.profile_version),
          })}
          disabled={rollback.isPending}
          onClick={() => rollback.mutate()}
        >
          <RotateCcw aria-hidden />
        </button>
      )}
      {error && (
        <span className="t-caption" role="alert">
          {error}
        </span>
      )}
    </li>
  );
}

// The wire vocabularies rendered through i18n; an unknown value (a newer
// server) renders verbatim rather than hiding the row.
function versionStatusLabel(
  t: ReturnType<typeof useT>,
  status: string,
): string {
  switch (status) {
    case "active":
      return t("voice.status.active");
    case "candidate":
      return t("voice.status.candidate");
    case "superseded":
      return t("voice.status.superseded");
    case "rejected":
      return t("voice.status.rejected");
    default:
      return status;
  }
}

function classificationLabel(
  t: ReturnType<typeof useT>,
  value: string,
): string {
  switch (value) {
    case "routine":
      return t("voice.classification.routine");
    case "material":
      return t("voice.classification.material");
    default:
      return value;
  }
}

function outcomeLabel(t: ReturnType<typeof useT>, value: string): string {
  switch (value) {
    case "auto_activated":
      return t("voice.outcome.autoActivated");
    case "review_required":
      return t("voice.outcome.reviewRequired");
    case "manually_activated":
      return t("voice.outcome.manuallyActivated");
    case "rejected":
      return t("voice.outcome.rejected");
    case "rollback":
      return t("voice.outcome.rollback");
    default:
      return value;
  }
}
