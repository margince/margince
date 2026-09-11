import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useRef, useState } from "react";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { Checkbox } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { useT } from "../i18n";
import { entityTimelineKeys } from "./activitykeys";
import { problemMessageOf, throwProblem } from "./common";

// Moving a filed message to the record it actually belongs to, extracted from
// compose.tsx unchanged.
//
// RELINK_KINDS is here rather than beside the composer because it is the
// vocabulary BOTH surfaces speak: the drawer files a message under one of these
// kinds, and the timeline moves one between them. A second spelling in either
// place is how the two stop agreeing about what a message can be filed under.
//
// MOVED, NOT REWRITTEN, for composeschedule.tsx's reason: compose.tsx sits
// under a frozen line cap the next slices have to fit inside, and a move that
// also rewrote would put the change under review out of reach.

// The link targets a relink can point at (relinkActivity's entity_type enum,
// minus `activity` — a relink never points at another activity). Reused by
// ComposeModal and TimelineActions so the whole surface speaks one vocabulary.
//
// An ARRAY with the type derived from it, rather than a bare union, because the
// picker below has to decide at RUNTIME whether a search hit is one of these.
export const RELINK_KINDS = [
  "person",
  "company",
  "deal",
  "lead",
  "project",
] as const;
export type RelinkKind = (typeof RELINK_KINDS)[number];

// Whether a cross-object search hit is something a message can be filed
// against. Asked as an ADMISSION rather than as a list of exclusions: the
// skip-list form named `activity` and `tag`, and silently admitted every type
// /search learned to return afterwards — which is how a product and an offer
// template became relink targets the moment the search enum widened. What this
// endpoint accepts is bounded and known; what search returns is not.
function isRelinkKind(type: string): type is RelinkKind {
  return (RELINK_KINDS as readonly string[]).includes(type);
}

// The relink target is chosen via cross-object search (/search covers every
// kind; the per-entity list endpoints don't all expose `q`). Each candidate's
// entity_type comes from its SearchResult.type, remembered here so the confirm
// can recover it — RecordPickerCandidate itself only carries {id,name}.
// Anything the relink enum does not name is dropped.
function useSearchTargets() {
  const kindById = useRef(new Map<string, RelinkKind>());
  const search = useCallback(
    async (q: string): Promise<RecordPickerCandidate[]> => {
      const { data, error } = await api.GET("/search", {
        params: { query: { q, limit: 10 } },
      });
      if (error) throwProblem(error);
      const out: RecordPickerCandidate[] = [];
      for (const result of data.data) {
        // Only what a relink may point at. An activity is the message itself, a
        // tag is a word rather than something a message can be about, and a
        // catalog row is neither — none of them is a record this can be filed
        // against, and the endpoint's own enum is what says so.
        if (!result.type || !isRelinkKind(result.type)) continue;
        kindById.current.set(result.id, result.type);
        out.push({ id: result.id, name: result.title ?? result.id });
      }
      return out;
    },
    [],
  );
  return { search, kindOf: (id: string) => kindById.current.get(id) ?? null };
}

// A 🟢 internal association (no autonomy dot): move or also-link a captured
// activity's typed link to the right person/company/deal/lead. Idempotent on the
// backend — re-relinking the same target is a no-op that still answers 200.
// `threadKey` is the activity's conversation key when it has one. With it the
// dialog offers to move the whole thread through `relinkThread`, which applies
// this same association to every message of the conversation the rep may
// edit, in one transaction — a mis-filed conversation is usually mis-filed
// whole.
// What one confirmed relink asks for, read at the moment the reader confirmed
// it rather than at whichever render the mutation's options were last armed on.
type RelinkRequest = Readonly<{
  activityId: string;
  threadKey?: string | null;
  target: RecordPickerCandidate;
  version?: number | null;
  thread: boolean;
  replace: boolean;
}>;

export function RelinkModal({
  activityId,
  activityVersion,
  threadKey,
  entityType,
  entityId,
  open,
  onClose,
}: Readonly<{
  activityId: string;
  // The version the reader's copy of the activity was read at, sent as
  // If-Match so a relink cannot overwrite a change nobody saw. Absent only
  // where the caller genuinely has none; the thread door takes no version at
  // all, since one cannot condition a move across many activities.
  activityVersion?: number | null;
  threadKey?: string | null;
  entityType: RelinkKind;
  entityId: string;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const { search, kindOf } = useSearchTargets();
  const [target, setTarget] = useState<RecordPickerCandidate | null>(null);
  const [replace, setReplace] = useState(false);
  const [wholeThread, setWholeThread] = useState(false);

  // What the confirm decided arrives as the mutation's VARIABLE — ALL of it,
  // including which activity and which conversation, because a mixture is worse
  // than either: a fresh `thread` read beside a stale `threadKey` falls through
  // the thread door into the single relink, where the version guard has already
  // been skipped on the strength of `thread` being true.
  //
  // Read through this closure each would be the value from the
  // render before the confirm was enabled, because react-query re-arms a
  // mutation's options in a passive effect: a version that landed just before
  // the click would refuse a relink that is perfectly valid, and a toggle
  // flipped at the same moment would move the wrong set. The remaining guard is
  // a real path and stays — `kindOf` answers from the search results, and a
  // target whose remembered kind was lost must be surfaced rather than relinked
  // to nothing.
  const mutation = useMutation({
    mutationFn: async ({
      activityId,
      threadKey,
      target,
      version,
      thread,
      replace,
    }: RelinkRequest) => {
      const kind = kindOf(target.id);
      if (!kind) {
        throwProblem({ title: t("compose.relinkTarget") });
      }
      // A relink of ONE activity conditions on the version this reader saw, and
      // a copy that arrived without one cannot make that claim. Refused by name
      // rather than through requireVersion's bare throw: the reader sees the
      // mutation's error, and "something went wrong" is not a thing anyone can
      // act on when the answer is "reopen it and try again".
      if (!thread && version == null) {
        throwProblem({ title: t("compose.relinkNoVersion") });
      }
      if (threadKey && thread) {
        const { data, error } = await api.POST("/activities/relink-thread", {
          params: { header: { "Idempotency-Key": crypto.randomUUID() } },
          body: {
            thread_key: threadKey,
            entity_type: kind,
            entity_id: target.id,
            replace_existing_of_type: replace,
          },
        });
        if (error) throwProblem(error);
        return data;
      }
      const { data, error } = await api.POST("/activities/{id}/relink", {
        params: {
          // The version the reader's copy was read at, so a relink cannot
          // overwrite a change nobody saw. requireVersion refuses the write
          // rather than sending it unpinned: unpinned is last-write-wins, and
          // the mutation's own error path is what tells the reader it did not
          // go through.
          //
          // The idempotency key travels INSIDE the precondition rather than in
          // a `header:` of its own: they are one slot, and written twice the
          // later wins.
          ...ifMatch(requireVersion(version ?? undefined), {
            "Idempotency-Key": crypto.randomUUID(),
          }),
          path: { id: activityId },
        },
        body: {
          entity_type: kind,
          entity_id: target.id,
          replace_existing_of_type: replace,
        },
      });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: () => {
      for (const queryKey of entityTimelineKeys(entityType, entityId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      // A relink is exactly the write that changes where a reply files, and the
      // composer's filing line reads the activity to say so. Without this the
      // line keeps naming the project the activity was moved AWAY from, which
      // is a wrong answer to the one question it exists to answer.
      queryClient.invalidateQueries({ queryKey: ["activity", activityId] });
      onClose();
    },
  });

  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t("compose.relinkTitle")}
      confirmLabel={t("compose.relinkConfirm")}
      confirmDisabled={!target}
      onConfirm={() =>
        target &&
        mutation.mutate({
          activityId,
          threadKey,
          target,
          version: activityVersion,
          thread: wholeThread,
          replace,
        })
      }
      pending={mutation.isPending}
      error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
    >
      <div className="compose-fields">
        <RecordPicker
          label={t("compose.relinkTarget")}
          searchTargets={search}
          onPick={setTarget}
          selected={target}
        />
        <Checkbox
          className="t-body"
          label={t("compose.relinkReplace")}
          checked={replace}
          onChange={(event) => setReplace(event.target.checked)}
        />
        <p className="t-caption">{t("compose.relinkReplaceHint")}</p>
        {threadKey && (
          <>
            <Checkbox
              className="t-body"
              label={t("compose.relinkThread")}
              checked={wholeThread}
              onChange={(event) => setWholeThread(event.target.checked)}
            />
            <p className="t-caption">{t("compose.relinkThreadHint")}</p>
          </>
        )}
      </div>
    </ConfirmModal>
  );
}

// Fill the form from a served draft, without ever clobbering a field the rep
// already edited.
//
// The reference, the disclosure and the reasons all describe the WORDS they
// were served with, so all three ride on exactly the condition that applies
// the body. A re-draft over text the rep already wrote keeps that text —
// adopting the newer reference would report a stranger's draft as the rep's
// own edit of it, the newer disclosure would credit a model with words a
// human typed, and the newer reasons would explain a draft nobody is looking
// at.
