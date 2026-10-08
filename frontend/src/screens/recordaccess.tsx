// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHO CAN SEE THIS RECORD, said in the header and changed there.
//
// One component for both records, because they are one question, and both
// headers answer it in the same place. It is a different setting from the
// company sidebar's "Private correspondence", which is a capture hold on a mail
// domain (counterparty-hold.tsx).
//
// The mark sits on the identity line rather than in the details rail: who may
// read a record is true of the RECORD, not of whichever tab is open, and the
// rail folds away. It is one chip among the record's other marks, with the
// answer, the switch and the way to the full list behind it, so nothing on the
// header moves or holds space for a reader who never asks.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { useRecordWriteRefusal } from "../app/capability";
import { navigate, navigateReplacing } from "../app/router";
import { Button } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { ErrorLine } from "../design-system/errorline";
import { Popover } from "../design-system/popover";
import { useToast } from "../design-system/toast";
import { VisibilityBadge } from "../design-system/visibility";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  isVersionSkewOf,
  problemMessageOf,
  throwProblem,
  useViewerId,
} from "./common";
import { useMemberName } from "./membernames";
import { invalidateRecord, recordWriteKeys } from "./recordwritekeys";
import "./recordaccess.css";

// The shape both records share, which is all this component reads. Spelled
// structurally rather than as `Contact | Company`: the two differ in every
// other field, and a union would make each read a narrowing.
type AccessibleRecord = Readonly<{
  id?: string;
  version?: number;
  visibility?: Audience;
  owner_id?: string | null;
  writable?: boolean;
  archived_at?: string | null;
}>;

type Audience = "workspace" | "owner";

// Which record this is, which decides the endpoint, the cache key and the
// sentences. A closed union rather than a string: a typo would otherwise
// invalidate a cache nobody watches and leave the header stale after a write.
type RecordKind = "contact" | "company";

// The sentences, per record: "this contact" and "this company", because a
// shared "this record" reads as system language on a page that names somebody
// by their own name. The two refusals are the ones each page already states
// for its other writes, so the header cannot give a reason the page does not.
const COPY = {
  contact: {
    title: "recordAccess.contact.title",
    shared: "recordAccess.contact.shared",
    privateYours: "recordAccess.contact.privateYours",
    privateOf: "recordAccess.contact.privateOf",
    privateOfOwner: "recordAccess.contact.privateOfOwner",
    published: "recordAccess.contact.published",
    madePrivate: "recordAccess.contact.madePrivate",
    leftYourAccess: "recordAccess.contact.leftYourAccess",
    archived: "contact.rail.archivedReadOnly",
    notYours: "contact.notYoursToChange",
  },
  company: {
    title: "recordAccess.company.title",
    shared: "recordAccess.company.shared",
    privateYours: "recordAccess.company.privateYours",
    privateOf: "recordAccess.company.privateOf",
    privateOfOwner: "recordAccess.company.privateOfOwner",
    published: "recordAccess.company.published",
    madePrivate: "recordAccess.company.madePrivate",
    leftYourAccess: "recordAccess.company.leftYourAccess",
    archived: "record.archivedReadOnly",
    notYours: "record.notYoursToChange",
  },
} as const satisfies Record<RecordKind, Record<string, MessageKey>>;

// The list a reader lands on once a write has taken the record out of their
// reach, which is where archiving one already takes them.
const LIST = { contact: "contacts", company: "companies" } as const;

/**
 * Who may read the record, as a chip that opens the answer and the switch.
 *
 * `record` is the row as the page read it, and its `version` rides the write
 * as If-Match. That is not politeness: the server's visibility column carries
 * a staleness guard, so a write built on a stale read is refused rather than
 * re-publishing a record somebody has just closed. Sending the version is how
 * this client never reaches that guard.
 */
export function RecordAccess({
  kind,
  record,
}: Readonly<{ kind: RecordKind; record: AccessibleRecord }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const copy = COPY[kind];
  // The page's own write rule (grant, seat, row, archive) rather than the
  // row's `writable` alone, which offered a read seat a switch it cannot use.
  const refusal = useRecordWriteRefusal(kind, record, {
    archived: t(copy.archived),
    notYours: t(copy.notYours),
  });
  const sentence = useAudienceSentence(kind, record);
  const viewerId = useViewerId();
  const failure = (error: unknown) =>
    isVersionSkewOf(error) ? t("edit.versionSkew") : problemMessageOf(error, t);
  const setVisibility = useMutation({
    // Every value the write needs is a VARIABLE, never a closure over the
    // render that drew the control: a click landing between the commit and the
    // passive effect that re-arms the options would otherwise run against the
    // previous render's record and write a version that has moved.
    mutationFn: async ({
      id,
      version,
      visibility,
    }: {
      id: string;
      version: AccessibleRecord["version"];
      visibility: Audience;
      mayLoseAccess: boolean;
    }) => {
      const path = kind === "contact" ? "/contacts/{id}" : "/companies/{id}";
      const { error } = await api.PATCH(path, {
        params: { path: { id }, ...ifMatch(requireVersion(version)) },
        body: { visibility },
      });
      if (error) {
        throwProblem(error);
      }
    },
    // The toast as well as the line in the panel, because the panel may be
    // closed by the time the refusal arrives.
    onError: async (error, { id }) => {
      toast.show(failure(error), { tone: "danger" });
      await invalidateRecord(queryClient, kind, id);
    },
    onSuccess: async (_, { id, visibility, mayLoseAccess }) => {
      if (mayLoseAccess && (await gone(kind, id))) {
        toast.show(t(copy.leftYourAccess));
        // Replacing, and forgetting the record's reads: Back must not return
        // the reader to a page drawn from a cache the server now refuses.
        navigateReplacing({ screen: LIST[kind] });
        for (const queryKey of recordWriteKeys(kind, id)) {
          queryClient.removeQueries({ queryKey });
        }
        await queryClient.invalidateQueries({ queryKey: [LIST[kind]] });
        return;
      }
      toast.show(
        t(visibility === "workspace" ? copy.published : copy.madePrivate),
      );
      await invalidateRecord(queryClient, kind, id);
    },
  });

  // A record whose visibility the server did not send says nothing rather
  // than guessing: the column is the reason a reader can see the row at all,
  // and "shared" drawn by default would be a claim this component cannot make.
  if (!record.visibility || !record.id || !sentence) {
    return null;
  }
  const id = record.id;
  const current = record.visibility;
  const ownerId = record.owner_id ?? undefined;
  return (
    <Popover
      label={
        <>
          <span className="sr-only">{t(copy.title)}: </span>
          <VisibilityBadge
            state={current === "owner" ? "private" : "workspace"}
            opens
          />
        </>
      }
    >
      <div className="record-access-panel">
        <p>{sentence}</p>
        {refusal ? (
          <p className="t-caption">{refusal}</p>
        ) : (
          <AudienceSwitch
            legend={t(copy.title)}
            current={current}
            owner={ownerStanding(ownerId, viewerId)}
            pending={setVisibility.isPending}
            error={
              setVisibility.isError ? failure(setVisibility.error) : undefined
            }
            onSave={(visibility) =>
              setVisibility.mutate({
                id,
                version: record.version,
                visibility,
                mayLoseAccess: visibility === "owner" && ownerId !== viewerId,
              })
            }
          />
        )}
        {/* The full answer (every colleague, and why) is the share screen's;
            this panel names the audience and switches it. */}
        <Button
          variant="link"
          onClick={() => navigate({ screen: "share", id: kind, id2: id })}
        >
          {t("recordAccess.manage")}
        </Button>
      </div>
    </Popover>
  );
}

// The switch holds its answer until Save: a radio group selects on every
// arrow key, so a keyboard reader moving through the answers would otherwise
// publish or narrow the record at each step. It lives inside the panel, so a
// closed panel drops an answer nobody saved, and a refusal is shown only for a
// Save pressed in this opening (the toast carried an earlier one).
function AudienceSwitch({
  legend,
  current,
  owner,
  pending,
  error,
  onSave,
}: Readonly<{
  legend: string;
  current: Audience;
  owner: OwnerStanding;
  pending: boolean;
  error: string | undefined;
  onSave: (visibility: Audience) => void;
}>) {
  const t = useT();
  const [picked, setPicked] = useState<Audience>();
  const [saveTried, setSaveTried] = useState(false);
  const save = useRef<HTMLButtonElement>(null);
  const answer = picked ?? current;
  // The pick is the stored answer once a Save lands and the record is read
  // back. Save then has nothing to save and is refused, and a refused button
  // drops the focus it held, so the answer's own radio takes it; the panel
  // stays open on the sentence it now says.
  useEffect(() => {
    if (picked === undefined || picked !== current || pending) {
      return;
    }
    save.current?.parentElement
      ?.querySelector<HTMLInputElement>(`input[value="${picked}"]`)
      ?.focus();
    setPicked(undefined);
  }, [pending, picked, current]);
  return (
    <>
      <ChoiceList<Audience>
        legend={legend}
        hideLegend
        disabled={pending}
        value={answer}
        choices={[
          {
            value: "owner",
            label: t("recordAccess.option.owner"),
            description: t(OWNER_HINT[owner]),
            // The server refuses owner-only on a record nobody owns: no seat
            // could read it.
            disabled: owner === "none" && current !== "owner",
          },
          {
            value: "workspace",
            label: t("recordAccess.option.workspace"),
          },
        ]}
        onChange={setPicked}
      />
      <Button
        ref={save}
        variant="primary"
        disabled={answer === current && !pending}
        pending={pending}
        onClick={() => {
          setSaveTried(true);
          onSave(answer);
        }}
      >
        {t("record.save")}
      </Button>
      {saveTried && error !== undefined && (
        <ErrorLine inline>{error}</ErrorLine>
      )}
    </>
  );
}

type OwnerStanding = "reader" | "other" | "none";

function ownerStanding(
  ownerId: string | undefined,
  viewerId: string | undefined,
): OwnerStanding {
  if (ownerId === undefined) {
    return "none";
  }
  return ownerId === viewerId ? "reader" : "other";
}

// What the owner's answer costs, said to the reader it costs: a colleague
// who is not the owner stops reading a private row unless a share reaches them.
const OWNER_HINT = {
  reader: "recordAccess.option.ownerHint",
  other: "recordAccess.option.ownerHintNotYours",
  none: "recordAccess.option.ownerNeeded",
} as const satisfies Record<OwnerStanding, MessageKey>;

// Whether the reader can still open the record: a private row answers to its
// owner and its shares alone, and whether a share reaches this reader is the
// server's to say.
// A probe that fails is NOT an answer of gone: the write landed, and a throw
// here would turn its success into a refusal. The page's own refetch then
// reports whatever the network is doing.
async function gone(kind: RecordKind, id: string): Promise<boolean> {
  const path = kind === "contact" ? "/contacts/{id}" : "/companies/{id}";
  try {
    const { response } = await api.GET(path, { params: { path: { id } } });
    return response.status === 404;
  } catch {
    return false;
  }
}

// Who may read the record, true for THIS reader. A private record open in
// front of anyone but its owner is open because it was shared with them or
// their team (an administrator's role does not lift owner-privacy), so the
// sentence names the owner and the share.
function useAudienceSentence(
  kind: RecordKind,
  record: AccessibleRecord,
): string | undefined {
  const t = useT();
  const viewerId = useViewerId();
  const copy = COPY[kind];
  const isPrivate = record.visibility === "owner";
  const ownerId = record.owner_id ?? undefined;
  const yours = viewerId !== undefined && ownerId === viewerId;
  // Named by id, batched with the facts strip's own request for the same
  // owner within the same tick, so this costs no extra round trip.
  const name = useMemberName(isPrivate && !yours ? ownerId : null);
  if (!record.visibility) {
    return undefined;
  }
  if (!isPrivate) {
    return t(copy.shared);
  }
  if (yours) {
    return t(copy.privateYours);
  }
  return name.data != null
    ? t(copy.privateOf, { owner: name.data })
    : t(copy.privateOfOwner);
}
