// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";

import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Button, EmptyState, Textarea } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { LoadMoreButton, problemMessageOf, throwProblem } from "./common";

type ConfirmSubmission = components["schemas"]["ConfirmSubmission"];

/**
 * What subjects proposed through their own confirm links, and who decides.
 *
 * The table has carried its resolution columns since it shipped and nothing
 * ever wrote them: a contact could type a correction into the link we mailed
 * them, the row was filed, and no colleague could list it, see it, or act on
 * it. Every correction anybody has ever sent is still waiting.
 *
 * A CORRECTION IS ONLY REVIEWABLE AS A COMPARISON. "She says Schmidt, we hold
 * Schmitt" is the decision; either half alone is not, which is why the row
 * shows the proposed value beside the field it is about rather than as a bare
 * string somebody has to go and check.
 *
 * Its own file because privacy.tsx is frozen at its length (fe-file-length
 * waivers), and a lane is a lane: this one owns its query, its rows and its
 * decision.
 *
 * It sits beneath the formal request queue on the settings page, because a
 * correction is the same act arriving informally: somebody typed it into the
 * link we mailed them rather than filing a rights request, and the officer
 * answering one is the officer answering the other.
 */
export function ConfirmSubmissionsPanel() {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  // `useCanWrite`, not `useCan`: the seat ceiling is enforced before RBAC, so a
  // read seat holding the grant is still refused every POST — the control it
  // was shown could only ever 403.
  const canDecide = useCanWrite("contact", "update");
  const [decidingId, setDecidingId] = useState<string | null>(null);
  const [note, setNote] = useState("");
  const [failure, setFailure] = useState("");

  // PAGED, because the queue is as long as the subjects make it. A limit with
  // no continuation answered the first page and said nothing about the rest, so
  // a reviewer who worked to the bottom of the list had seen the oldest fifty
  // and none of what arrived after — and the screen gave no sign a tail
  // existed. The screen is what a reviewer actually works, so the fix has to
  // reach here and not only the wire.
  const query = useInfiniteQuery({
    queryKey: ["confirm-submissions"],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/confirm-submissions", {
        params: { query: { resolved: false, cursor: pageParam ?? undefined } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });

  const resolve = useMutation({
    // A VARIABLE, never a closure over render state: the row being decided and
    // the note typed about it both travel with the call, so a re-render between
    // the click and the response cannot retarget it.
    mutationFn: async (command: {
      id: string;
      resolution: "accepted" | "rejected";
      note: string;
    }) => {
      const { data, error } = await api.POST(
        "/confirm-submissions/{id}/resolve",
        {
          params: { path: { id: command.id } },
          body: {
            resolution: command.resolution,
            note: command.note || undefined,
          },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: async () => {
      setDecidingId(null);
      setNote("");
      setFailure("");
      await queryClient.invalidateQueries({
        queryKey: ["confirm-submissions"],
      });
      // The contact's own card too: an accepted correction wrote a field, and a
      // page still showing the old value would disagree with the record.
      await queryClient.invalidateQueries({ queryKey: ["contact"] });
    },
    onError: (err: unknown) => setFailure(problemMessageOf(err, t)),
  });

  const tz = viewerZone();
  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];
  return (
    <Panel title={t("privacy.corrections")}>
      <PanelBody>
        <PanelIntro>{t("privacy.correctionsSub")}</PanelIntro>
        {failure ? <ErrorLine>{failure}</ErrorLine> : null}
        {/* A FAILED READ IS NOT AN EMPTY QUEUE. Coercing an undefined answer
            to [] told the reviewer nothing was waiting when the read had in
            fact failed, which is the one wrong thing a work queue can say. */}
        <ErrorLine error={query.error} />
        {!query.isError && rows.length === 0 ? (
          <EmptyState title={t("privacy.correctionsEmpty")}>
            {t("privacy.correctionsEmptySub")}
          </EmptyState>
        ) : (
          <ul>
            {rows.map((row) => (
              <CorrectionRow
                key={row.id}
                row={row}
                locale={locale}
                tz={tz}
                canDecide={canDecide}
                deciding={decidingId === row.id}
                note={note}
                pending={resolve.isPending}
                onNote={setNote}
                onOpen={() => {
                  setDecidingId(row.id);
                  setNote("");
                  setFailure("");
                }}
                onDecide={(resolution) =>
                  resolve.mutate({ id: row.id, resolution, note })
                }
              />
            ))}
            <LoadMoreButton query={query} />
          </ul>
        )}
      </PanelBody>
    </Panel>
  );
}

/**
 * One proposal, and what the reviewer needs to answer it.
 *
 * THE NOTE IS OFFERED ON BOTH ANSWERS and matters most on a rejection. An
 * accepted correction explains itself — the field now holds what the subject
 * said — but "we did not change it" records no reason at all, and the subject
 * who asked is entitled to one when they ask again.
 */
function CorrectionRow({
  row,
  locale,
  tz,
  canDecide,
  deciding,
  note,
  pending,
  onNote,
  onOpen,
  onDecide,
}: Readonly<{
  row: ConfirmSubmission;
  locale: ReturnType<typeof useLocale>["locale"];
  tz: string;
  canDecide: boolean;
  deciding: boolean;
  note: string;
  pending: boolean;
  onNote: (value: string) => void;
  onOpen: () => void;
  onDecide: (resolution: "accepted" | "rejected") => void;
}>) {
  const t = useT();
  return (
    <li>
      <div>
        {/* WHO, first. This queue spans every contact, and two of them
            proposing the same title on the same day are indistinguishable
            without it — while accepting either changes a different record. */}
        <span>{row.contact_name ?? t("privacy.correctionUnnamed")}</span>
        <span>{row.field ?? t("privacy.correctionRemoval")}</span>
        {/* The subject's own words. A correction shown without them is a
            decision nobody can make. */}
        {/* BOTH HALVES. A correction is only reviewable as a comparison —
            "she says Schmidt, we hold Schmitt" is the decision, and the
            proposal alone is not. */}
        {row.proposed_value ? (
          <span>
            {/* Not a catalog key: an arrow between two values carries no
                words to translate, and three identical entries read as a
                translation nobody did. */}
            {row.current_value
              ? `${row.current_value} → ${row.proposed_value}`
              : row.proposed_value}
          </span>
        ) : null}
      </div>
      <span>{formatDate(row.submitted_at, locale, tz)}</span>
      {canDecide && !deciding ? (
        <Button onClick={onOpen}>{t("privacy.correctionDecide")}</Button>
      ) : null}
      {deciding ? (
        <div>
          <Textarea
            value={note}
            onChange={(e) => onNote(e.target.value)}
            placeholder={t("privacy.correctionNote")}
            maxLength={500}
          />
          {/* The queue's own action row, shared with the request lane above
              it: two verbs a hand apart should sit a hand apart on
              both. */}
          <div className="dsr-actions">
            <Button disabled={pending} onClick={() => onDecide("accepted")}>
              {/* A REMOVAL IS ACKNOWLEDGED, not performed here. Accepting one
                  records that somebody read it; what the contact asked for is
                  a rights case, opened when the proposal arrived, and
                  answering it is that case's business. A button reading
                  "accept and update" would claim this removed them. */}
              {row.field
                ? t("privacy.correctionAccept")
                : t("privacy.correctionAcknowledge")}
            </Button>
            <Button
              variant="ghost"
              disabled={pending}
              onClick={() => onDecide("rejected")}
            >
              {t("privacy.correctionReject")}
            </Button>
          </div>
        </div>
      ) : null}
    </li>
  );
}
