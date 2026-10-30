import { useInfiniteQuery, useMutation, useQuery } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Card, Disclosure } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { PanelBody, PanelGroupHead } from "../../design-system/panel";
import { useLastSubject } from "../../design-system/presence";
import { QueryGate } from "../../design-system/querystates";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatNumber, identifierNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { parseVoiceInsights, VoiceInsights } from "./voice-insights";
import { reviewReasonText } from "./voice-review-reason";
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

// One paged read of the version history, shared by every part of the page
// that needs it: the hero and the candidate review read the first page, the
// history table the pages a reader loads.
export function useVoiceVersions(profileId: string | undefined) {
  return useInfiniteQuery({
    queryKey: ["voice-versions", profileId],
    enabled: Boolean(profileId),
    initialPageParam: "",
    queryFn: async ({ pageParam }): Promise<VersionsPage> => {
      const data = unwrap(
        await api.GET("/voice-profiles/{id}/versions", {
          params: {
            path: { id: profileId ?? "" },
            query: pageParam ? { cursor: pageParam } : {},
          },
        }),
      );
      return { items: data.data, next: data.page.next_cursor ?? null };
    },
    getNextPageParam: (page) => page.next ?? undefined,
  });
}

// The versions every loaded page holds, newest first, each once.
export function loadedVersions(
  pages: readonly VersionsPage[] | undefined,
): VoiceProfileVersion[] {
  const all = (pages ?? []).reduce<VoiceProfileVersion[]>(
    (merged, page) => mergeById(merged, page.items),
    [],
  );
  return all.sort((a, b) => b.profile_version - a.profile_version);
}

export function useVoiceLearning(profileId: string) {
  return useQuery({
    queryKey: ["voice-learning", profileId],
    queryFn: async (): Promise<VoiceLearningSummary> => {
      return unwrap(
        await api.GET("/voice-profiles/{id}/learning", {
          params: { path: { id: profileId } },
        }),
      );
    },
  });
}

// A candidate never replaces the active voice silently: the owner applies
// or rejects it, with the version itself and the evaluator's reasons in view.
export function CandidateBanner({
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
            {candidate.review_reasons.map((reason, index) => (
              <li key={reason}>
                {reviewReasonText(
                  t,
                  locale,
                  reason,
                  candidate.evaluation.review_reason_details === undefined
                    ? undefined
                    : (candidate.evaluation.review_reason_details[index] ??
                        null),
                )}
              </li>
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

// Stands straight in the builds panel so its table bleeds, under its own group head.
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
  const [restoring, setRestoring] = useState<VoiceProfileVersion | null>(null);
  const rolledBack = useRef(false);
  const headingId = useId();
  const versions = useVoiceVersions(profileId);
  const learning = useVoiceLearning(profileId);
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
      rolledBack.current = true;
      setRestoring(null);
      onChanged();
    },
  });
  const asked = useLastSubject(restoring);
  const restoringLabel = identifierNumber(asked?.profile_version ?? 0);

  return (
    <>
      <PanelGroupHead
        level="h3"
        title={
          <span id={headingId} tabIndex={-1}>
            {t("voice.history.label")}
          </span>
        }
      />
      <QueryGate query={versions} pendingLabel={t("voice.history.label")}>
        {(loaded) => {
          const allVersions = loadedVersions(loaded.pages);
          return allVersions.length === 0 ? (
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
                    rolledBack.current = false;
                    setRestoring(version);
                  },
                })}
                rows={allVersions}
                rowKey={(version) => version.id}
              />
              {versions.hasNextPage && (
                <PanelBody>
                  <Button
                    disabled={versions.isFetchingNextPage}
                    onClick={() => versions.fetchNextPage()}
                  >
                    {t("voice.history.loadMore")}
                  </Button>
                </PanelBody>
              )}
            </>
          );
        }}
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
        returnFocusTo={() =>
          rolledBack.current ? document.getElementById(headingId) : null
        }
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
