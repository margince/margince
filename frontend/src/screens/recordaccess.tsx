// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHO CAN SEE THIS RECORD, said in the header and changed there.
//
// One component for both records, because they are one question. The contact
// header carried this and the company header carried nothing — so the same
// fact lived in two places on one product: a contact said "Only you" beside
// its name, and a capture-private company said nothing at all while its
// sidebar showed a "Private correspondence" section that is a DIFFERENT
// setting (a capture hold on a mail domain, counterparty-hold.tsx). A reader
// who learned the contact page then read the company page was told the account
// was shared, by omission, when it was not.
//
// The mark and the verb sit on the identity line rather than in the details
// rail: who may read a record is true of the RECORD, not of whichever tab is
// open, and the rail folds away.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useId } from "react";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { Button } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { useTooltip } from "../design-system/tooltip";
import { VisibilityLine } from "../design-system/visibility";
import { useT } from "../i18n";
import { isVersionSkewOf, problemMessageOf, throwProblem } from "./common";
import "./recordaccess.css";

// The shape both records share, which is all this component reads. Spelled
// structurally rather than as `Contact | Company`: the two differ in every
// other field, and a union would make each read a narrowing.
type AccessibleRecord = Readonly<{
  id?: string;
  version?: number;
  visibility?: "workspace" | "owner";
  writable?: boolean;
  archived_at?: string | null;
}>;

// Which record this is, which decides the endpoint, the cache key and the
// sentences. A closed union rather than a string: a typo would otherwise
// invalidate a cache nobody watches and leave the header stale after a write.
type RecordKind = "contact" | "company";

// The sentence under the mark, per record. Two records, two nouns — "this
// contact" and "this account" — because a shared "this record" reads as
// system language on a page that names somebody by their own name.
const COPY = {
  contact: {
    title: "recordAccess.contact.title",
    private: "recordAccess.contact.privateToYou",
    shared: "recordAccess.contact.shared",
    privateTip: "recordAccess.contact.privateTip",
    share: "recordAccess.contact.share",
    makePrivate: "recordAccess.contact.makePrivate",
    published: "recordAccess.contact.published",
    madePrivate: "recordAccess.contact.madePrivate",
  },
  company: {
    title: "recordAccess.company.title",
    private: "recordAccess.company.privateToYou",
    shared: "recordAccess.company.shared",
    privateTip: "recordAccess.company.privateTip",
    share: "recordAccess.company.share",
    makePrivate: "recordAccess.company.makePrivate",
    published: "recordAccess.company.published",
    madePrivate: "recordAccess.company.madePrivate",
  },
} as const;

// The 360 query each record page holds, so the write invalidates the read the
// header is drawn from rather than a list the page is not showing.
const QUERY_KEY = { contact: "contact360", company: "company360" } as const;

/**
 * Visibility stays beside the owner even when the details rail is closed.
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
  const descriptionId = useId();
  const copy = COPY[kind];
  const setVisibility = useMutation({
    // Every value the write needs is a VARIABLE, never a closure over the
    // render that drew the button: a click landing between the commit and the
    // passive effect that re-arms the options would otherwise run against the
    // previous render's record and write a version that has moved.
    mutationFn: async ({
      id,
      version,
      visibility,
    }: {
      id: string;
      version: AccessibleRecord["version"];
      visibility: "workspace" | "owner";
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
  if (!record.visibility || !record.id) {
    return null;
  }
  const isPrivate = record.visibility === "owner";
  const description = t(isPrivate ? copy.private : copy.shared);
  const mayChange = Boolean(record.writable) && !record.archived_at;
  const id = record.id;
  return (
    <section className="record-access" aria-label={t(copy.title)}>
      {/* The mark explains itself on hover and on focus: what "Only you"
          means, and that the verb beside it is how to change it. A native
          title reached a pointer alone and only after a pause. */}
      <AccessTip text={t(isPrivate ? copy.privateTip : copy.shared)}>
        {(tipId) => (
          <VisibilityLine
            state={isPrivate ? "private" : "team"}
            action={
              mayChange && (
                <Button
                  variant="link"
                  className="record-access-action"
                  aria-describedby={describedBy(descriptionId, tipId)}
                  pending={setVisibility.isPending}
                  onClick={() =>
                    setVisibility.mutate({
                      id,
                      version: record.version,
                      visibility: isPrivate ? "workspace" : "owner",
                    })
                  }
                >
                  {t(isPrivate ? copy.share : copy.makePrivate)}
                </Button>
              )
            }
          />
        )}
      </AccessTip>
      <span id={descriptionId} className="sr-only">
        {description}
      </span>
      {setVisibility.isError && (
        <span role="alert" className="form-error">
          {isVersionSkewOf(setVisibility.error)
            ? t("edit.versionSkew")
            : problemMessageOf(setVisibility.error, t)}
        </span>
      )}
    </section>
  );
}

// Both descriptions on the one control: the sr-only sentence it always
// carries and the tip while it is open.
function describedBy(...ids: (string | undefined)[]): string | undefined {
  const joined = ids.filter(Boolean).join(" ");
  return joined || undefined;
}

// The tooltip's anchor: one span around the line, so the badge and the verb
// share the one explanation rather than each hanging its own. The tip's id is
// handed to the children, because `aria-describedby` does not inherit: set on
// the span alone, the button inside it stayed undescribed for a screen reader.
function AccessTip({
  text,
  children,
}: Readonly<{ text: string; children: (tipId?: string) => ReactNode }>) {
  const { ref, trigger, tip } = useTooltip<HTMLSpanElement>(text);
  return (
    <span className="record-access-tip" ref={ref} {...trigger}>
      {children(trigger["aria-describedby"])}
      {tip}
    </span>
  );
}
