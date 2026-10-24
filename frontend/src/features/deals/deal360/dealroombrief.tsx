// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The room brief: what the room's buyers did and what is waiting on the rep.
// It is written from the participants, documents and threads the tab draws,
// and every sentence names what it was written from.

import { useQuery } from "@tanstack/react-query";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { WrittenBy } from "../../../components/citations.list";
import { AiPending } from "../../../design-system/aipending";
import { Badge } from "../../../design-system/atoms";
import { Panel, PanelBody } from "../../../design-system/panel";
import { useT } from "../../../i18n";
import { unwrap } from "../../../lib/problem";
import { useParticipants } from "../dealroomaccess";
import { ROOM_BRIEF_PREFIX } from "../dealroombrief.key";
import { useRoomThreads } from "../dealroomconversation";
import { useRoomDocuments } from "../dealroomdocuments";
import "./dealroomtab.css";

type DealRoomBrief = components["schemas"]["DealRoomBrief"];
type Evidence = components["schemas"]["DealRoomBriefEvidence"];

// A model writes the brief on each read, so a window focus does not read it
// again. A minute is as fresh as a reader can tell.
const BRIEF_STALE_MS = 60_000;

export function useRoomBrief(roomId: string) {
  const t = useT();
  return useQuery({
    queryKey: [...ROOM_BRIEF_PREFIX, roomId],
    staleTime: BRIEF_STALE_MS,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      return unwrap(
        await api.GET("/deal-rooms/{id}/brief", {
          params: { path: { id: roomId } },
        }),
        t,
      );
    },
  });
}

/**
 * The brief as an AI panel. It draws nothing for a room with nothing to say,
 * or when the brief cannot be read. The records it sums up are on the tab
 * beside it either way.
 */
export function RoomBrief({ roomId }: Readonly<{ roomId: string }>) {
  const t = useT();
  const brief = useRoomBrief(roomId);
  const sentences = brief.data?.sentences ?? [];
  if (brief.isError || (brief.isSuccess && sentences.length === 0)) {
    return null;
  }
  return (
    <Panel
      title={t("roomtab.brief.title")}
      tone="ai"
      titleAction={<Badge tone="ai">{t("co.assistant.aiTag")}</Badge>}
      footer={brief.data ? <WrittenBy by={brief.data.generated_by} /> : null}
    >
      <PanelBody>
        {brief.data ? (
          <BriefSentences roomId={roomId} brief={brief.data} />
        ) : (
          <AiPending label={t("roomtab.brief.pending")} lines={2} />
        )}
      </PanelBody>
    </Panel>
  );
}

function BriefSentences({
  roomId,
  brief,
}: Readonly<{ roomId: string; brief: DealRoomBrief }>) {
  const t = useT();
  const { settled, nameOf } = useEvidenceNames(roomId);
  // A sentence never shows without the line that says where it came from, so
  // the brief waits for the reads that name its sources.
  if (!settled) {
    return <AiPending label={t("roomtab.brief.pending")} lines={2} />;
  }
  return (
    <ul className="roombrief">
      {brief.sentences.map((sentence) => {
        const sources = sentence.evidence.map(
          (evidence) => nameOf(evidence) ?? t("roomtab.brief.unknownSource"),
        );
        return (
          <li key={sentence.text}>
            <p>{sentence.text}</p>
            <p className="t-caption">
              {t("roomtab.brief.from", {
                sources: [...new Set(sources)].join(", "),
              })}
            </p>
          </li>
        );
      })}
    </ul>
  );
}

// What a citation names, read from the queries the tab already holds. So the
// brief costs no request of its own to say where it came from.
function useEvidenceNames(roomId: string) {
  const t = useT();
  const participantsRead = useParticipants(roomId);
  const documentsRead = useRoomDocuments(roomId);
  const threadsRead = useRoomThreads(roomId);
  const participants = participantsRead.data?.data ?? [];
  const documents = documentsRead.data?.data ?? [];
  const threads = threadsRead.data?.data ?? [];
  const names = new Map<string, string>();
  for (const p of participants) {
    names.set(`participant:${p.id}`, p.full_name);
  }
  for (const doc of documents) {
    names.set(`document:${doc.id}`, doc.title);
  }
  const titles = new Map(documents.map((doc) => [doc.id, doc.title]));
  for (const thread of threads) {
    const about = thread.document_id
      ? titles.get(thread.document_id)
      : undefined;
    names.set(
      `thread:${thread.id}`,
      about
        ? t("threads.about", { title: about })
        : t("roomtab.brief.roomThread"),
    );
  }
  return {
    // Settled once each read answered or failed; a failed read leaves its
    // citations to the generic source label rather than holding the brief.
    settled: [participantsRead, documentsRead, threadsRead].every(
      (read) => !read.isPending,
    ),
    nameOf: (evidence: Evidence) =>
      names.get(`${evidence.entity_type}:${evidence.entity_id}`),
  };
}
