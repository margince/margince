import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Button, Card, Disclosure } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber, identifierNumber } from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf, QueryGate, unwrap } from "./common";
import { parseVoiceInsights, VoiceInsights } from "./voice-insights";
import { changeColumns, versionColumns } from "./voice-versions.tables";
import "./voice-dna.css";

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
      const data = unwrap(
        await api.GET("/voice-profiles/{id}/versions", {
          params: { path: { id: profileId as string } },
        }),
      );
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
// says nothing to the colleague being asked to decide. Each known shape is stated
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
      unwrap(
        await api.POST(path, {
          params: {
            path: { id: profileId, profileVersion: candidate.profile_version },
            header: { "If-Match": String(candidate.version) },
          },
        }),
      );
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
      <p>{t("voice.candidate.whatItIs")}</p>
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
          <p className="vdna-label">{t("voice.candidate.concernsLabel")}</p>
          <ul className="vdna-reasons">
            {candidate.review_reasons.map((reason) => (
              <li key={reason}>{reviewReasonText(t, locale, reason)}</li>
            ))}
          </ul>
        </>
      )}
      <p className="t-caption">{t("voice.candidate.applyHint")}</p>
      {error && <ErrorLine>{error}</ErrorLine>}
      {canEdit && (
        <div className="vdna-candidate-acts">
          <Button
            variant="primary"
            disabled={transition.isPending}
            onClick={() => transition.mutate("apply")}
          >
            {t("voice.candidate.apply")}
          </Button>
          <Button
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

// Stands straight in the builds panel so its table bleeds; the group head
// above it names it.
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
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const [versionCursor, setVersionCursor] = useState<string | undefined>();
  const [allVersions, setAllVersions] = useState<VoiceProfileVersion[]>([]);
  const [restoring, setRestoring] = useState<VoiceProfileVersion | null>(null);
  const versions = useQuery({
    queryKey: ["voice-versions", profileId, versionCursor ?? ""],
    queryFn: async (): Promise<VersionsPage> => {
      const data = unwrap(
        await api.GET("/voice-profiles/{id}/versions", {
          params: {
            path: { id: profileId },
            query: versionCursor ? { cursor: versionCursor } : {},
          },
        }),
      );
      setAllVersions((prev) => mergeById(prev, data.data));
      return { items: data.data, next: data.page.next_cursor ?? null };
    },
  });
  const learning = useQuery({
    queryKey: ["voice-learning", profileId],
    queryFn: async (): Promise<VoiceLearningSummary> => {
      return unwrap(
        await api.GET("/voice-profiles/{id}/learning", {
          params: { path: { id: profileId } },
        }),
      );
    },
  });
  const rollback = useMutation({
    mutationFn: async (profileVersion: number) => {
      unwrap(
        await api.POST(
          "/voice-profiles/{id}/versions/{profileVersion}/rollback",
          { params: { path: { id: profileId, profileVersion } } },
        ),
      );
    },
    onSuccess: () => {
      setRestoring(null);
      onChanged();
    },
  });
  const restoringLabel = identifierNumber(restoring?.profile_version ?? 0);

  return (
    <>
      <QueryGate query={versions} pendingLabel={t("voice.history.label")}>
        {(page) =>
          allVersions.length === 0 ? (
            <PanelBody>
              <SurfaceState
                state="empty"
                emptyLabel={t("voice.history.empty")}
                loadingLabel={t("voice.history.label")}
              >
                {null}
              </SurfaceState>
            </PanelBody>
          ) : (
            <>
              <DataTable
                bleed
                fold
                label={t("voice.history.label")}
                columns={versionColumns({
                  t,
                  plural,
                  locale,
                  zone,
                  canEdit,
                  onRestore: (version) => {
                    rollback.reset();
                    setRestoring(version);
                  },
                })}
                rows={[...allVersions].sort(
                  (a, b) => b.profile_version - a.profile_version,
                )}
                rowKey={(version) => version.id}
              />
              {page.next && (
                <PanelBody>
                  <Button
                    onClick={() => setVersionCursor(page.next ?? undefined)}
                  >
                    {t("voice.history.loadMore")}
                  </Button>
                </PanelBody>
              )}
            </>
          )
        }
      </QueryGate>
      <PanelBody>
        <QueryGate query={learning} pendingLabel={t("voice.history.label")}>
          {(summary) => (
            <p className="t-caption">
              {t("voice.history.learning", {
                drafted: formatNumber(summary.drafted, locale),
                edited: formatNumber(summary.edited_sent, locale),
                rejected: formatNumber(summary.rejected, locale),
              })}
            </p>
          )}
        </QueryGate>
      </PanelBody>
      <ConfirmModal
        open={restoring !== null}
        title={t("voice.history.rollbackConfirm.title", { n: restoringLabel })}
        confirmLabel={t("voice.history.rollback", { n: restoringLabel })}
        pending={rollback.isPending}
        error={rollback.isError ? problemMessageOf(rollback.error, t) : null}
        onClose={() => setRestoring(null)}
        onConfirm={() =>
          restoring && rollback.mutate(restoring.profile_version)
        }
      >
        <p>{t("voice.history.rollbackConfirm.body", { n: restoringLabel })}</p>
      </ConfirmModal>
    </>
  );
}

// What each build changed, kept behind a Disclosure because nothing here acts.
// Its own cursor pages it apart from the versions.
export function VoiceChangeLog({ profileId }: Readonly<{ profileId: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const [deltaCursor, setDeltaCursor] = useState<string | undefined>();
  const [allDeltas, setAllDeltas] = useState<VoiceProfileDelta[]>([]);
  const deltas = useQuery({
    queryKey: ["voice-deltas", profileId, deltaCursor ?? ""],
    queryFn: async (): Promise<DeltasPage> => {
      const data = unwrap(
        await api.GET("/voice-profiles/{id}/deltas", {
          params: {
            path: { id: profileId },
            query: deltaCursor ? { cursor: deltaCursor } : {},
          },
        }),
      );
      setAllDeltas((prev) => mergeById(prev, data.data));
      return { items: data.data, next: data.page.next_cursor ?? null };
    },
  });
  return (
    <QueryGate query={deltas} pendingLabel={t("voice.history.deltasLabel")}>
      {(page) =>
        allDeltas.length === 0 ? (
          <p>{t("voice.history.deltasEmpty")}</p>
        ) : (
          <div className="form-stack">
            <DataTable
              fold
              label={t("voice.history.deltasLabel")}
              columns={changeColumns({ t, plural, locale, zone })}
              rows={[...allDeltas].sort((a, b) => b.to_version - a.to_version)}
              rowKey={(delta) => delta.id}
            />
            {page.next && (
              <Button onClick={() => setDeltaCursor(page.next ?? undefined)}>
                {t("voice.history.loadMore")}
              </Button>
            )}
          </div>
        )
      }
    </QueryGate>
  );
}
