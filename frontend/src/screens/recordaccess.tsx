// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHO CAN SEE THIS RECORD, said in the header and changed there.
//
// One component for both records, because they are one question. The contact
// header carried this and the company header carried nothing — so the same
// fact lived in two places on one product: a contact said who could read it
// beside its name, and a capture-private company said nothing at all while its
// sidebar showed a "Private correspondence" section that is a DIFFERENT
// setting (a capture hold on a mail domain, counterparty-hold.tsx). A reader
// who learned the contact page then read the company page was told the account
// was shared, by omission, when it was not.
//
// The mark sits on the identity line rather than in the details rail: who may
// read a record is true of the RECORD, not of whichever tab is open, and the
// rail folds away. It is one chip among the record's other marks, with the
// answer, the switch and the way to the full list behind it: verbs revealed on
// hover beside the mark moved the header under the pointer and held empty
// space for every other reader.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { useRecordWriteRefusal } from "../app/capability";
import { navigate } from "../app/router";
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
import { memberName, useRosterNames } from "./roster";
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
    archived: "record.archivedReadOnly",
    notYours: "record.notYoursToChange",
  },
} as const satisfies Record<RecordKind, Record<string, MessageKey>>;

// The 360 query each record page holds, so the write invalidates the read the
// header is drawn from rather than a list the page is not showing.
const QUERY_KEY = { contact: "contact360", company: "company360" } as const;

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
    }) => {
      const path = kind === "contact" ? "/contacts/{id}" : "/companies/{id}";
      const { error } = await api.PATCH(path, {
        params: { path: { id }, ...ifMatch(requireVersion(version)) },
        body: { visibility },
      });
      if (error) {
        throwProblem(error);
      }
      return { id, visibility };
    },
    onError: async (error, { id }) => {
      if (isVersionSkewOf(error)) {
        await queryClient.invalidateQueries({
          queryKey: [QUERY_KEY[kind], id],
        });
      }
    },
    onSuccess: async ({ id, visibility }) => {
      toast.show(
        t(visibility === "workspace" ? copy.published : copy.madePrivate),
      );
      await queryClient.invalidateQueries({ queryKey: [QUERY_KEY[kind], id] });
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
  // The answer just given, until the refetch confirms it: a radio that stays
  // on the old answer for a round trip reads as a press that missed.
  const chosen =
    (setVisibility.isPending && setVisibility.variables?.visibility) || current;
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
          <ChoiceList<Audience>
            legend={t(copy.title)}
            hideLegend
            value={chosen}
            choices={[
              {
                value: "owner",
                label: t("recordAccess.option.owner"),
                description: t("recordAccess.option.ownerHint"),
              },
              {
                value: "workspace",
                label: t("recordAccess.option.workspace"),
              },
            ]}
            onChange={(visibility) => {
              if (setVisibility.isPending || visibility === current) {
                return;
              }
              setVisibility.mutate({ id, version: record.version, visibility });
            }}
          />
        )}
        {setVisibility.isError && (
          <ErrorLine inline>
            {isVersionSkewOf(setVisibility.error)
              ? t("edit.versionSkew")
              : problemMessageOf(setVisibility.error, t)}
          </ErrorLine>
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

// Who may read the record, true for THIS reader. A private record open in
// front of anyone but its owner is open because it was shared with them — an
// administrator's role does not lift owner-privacy — so "Only you" is false.
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
  // The cache entry the facts strip already names the owner from, so this
  // costs no request on either record page.
  const roster = useRosterNames("user", isPrivate && !yours && !!ownerId);
  if (!record.visibility) {
    return undefined;
  }
  if (!isPrivate) {
    return t(copy.shared);
  }
  if (yours) {
    return t(copy.privateYours);
  }
  const owner = ownerId && memberName(roster.data, ownerId);
  return owner ? t(copy.privateOf, { owner }) : t(copy.privateOfOwner);
}
