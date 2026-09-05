// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Avatar, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CompanyLogo } from "../design-system/companylogo";
import { FileDropzone } from "../design-system/filedropzone";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import "./companymark.css";

type CompanyProfile = components["schemas"]["CompanyProfile"];

// What the picker offers, and what the server will decode. Both lists say the
// same thing, and this one is the browser's filter only: a drop is not filtered
// by it, and the server refuses what it cannot read either way.
const ACCEPTED_IMAGES =
  "image/png,image/jpeg,image/gif,image/webp,image/svg+xml,image/x-icon";

/**
 * The installation's own marks: what they are now, and what a person can do
 * about each.
 *
 * TWO of them, because the sidebar shows the company at two widths. The wide
 * mark heads the panel while it is open; the square icon stands in the 56px
 * rail when it is collapsed, where a wordmark is a row of illegible strokes.
 * They are chosen separately and cleared separately, and a company that fills
 * only the wide slot keeps drawing that mark in both — which is what every
 * installation did before the second slot existed.
 *
 * The wide mark is normally the one the website read resolved from the
 * company's own site. This is the other door — for an installation whose site
 * declares no icon, and for the read that resolved the wrong picture. Uploading
 * takes the field: while a person's own mark stands, a later read leaves it
 * alone. Removing gives it back, so the record returns to its monogram and the
 * next read may resolve one again. Nothing but an upload ever fills the square
 * slot, so it has no read to hold off.
 */
export function CompanyMark({
  profile,
  canEdit,
}: Readonly<{ profile: CompanyProfile; canEdit: boolean }>) {
  const t = useT();
  const client = useQueryClient();
  // The response IS the new profile, so it is written straight into the entry
  // the shell's brand block and this card both read. A refetch would ask the
  // server a question it just answered, and the rail would wear the old mark
  // until it came back.
  const settle = (next: CompanyProfile) => {
    client.setQueryData(["company"], next);
  };
  const wide = useMarkWrites("/v1/company/logo", deleteWideMark, settle);
  const icon = useMarkWrites("/v1/company/logo/icon", deleteIconMark, settle);

  return (
    <div className="company-mark">
      <div className="company-mark-body">
        <b>{t("settings.companyMark")}</b>
        <p className="t-caption">{t("settings.companyMarkIntro")}</p>
        <div className="company-mark-slots">
          <MarkSlot
            profile={profile}
            canEdit={canEdit}
            src={profile.logo_url}
            name={t("settings.companyMarkWide")}
            status={
              profile.logo_url
                ? t("settings.companyMarkWidePresent")
                : t("settings.companyMarkWideNone")
            }
            hint={t("settings.companyMarkWideHint")}
            emptyLabel={t("settings.companyMarkEmpty")}
            verbs={{
              add: t("settings.companyMarkAddWide"),
              replace: t("settings.companyMarkReplaceWide"),
              remove: t("settings.companyMarkRemoveWide"),
            }}
            writes={wide}
          />
          <MarkSlot
            profile={profile}
            canEdit={canEdit}
            src={profile.logo_icon_url}
            square
            name={t("settings.companyMarkIcon")}
            status={
              profile.logo_icon_url
                ? t("settings.companyMarkIconPresent")
                : t("settings.companyMarkIconNone")
            }
            hint={t("settings.companyMarkIconHint")}
            emptyLabel={t("settings.companyMarkIconEmpty")}
            verbs={{
              add: t("settings.companyMarkAddIcon"),
              replace: t("settings.companyMarkReplaceIcon"),
              remove: t("settings.companyMarkRemoveIcon"),
            }}
            writes={icon}
          />
        </div>
      </div>
    </div>
  );
}

// The removals, spelled at module scope because the typed client keys on the
// path LITERAL: a variable would leave the response `unknown` and the profile
// this write settles unchecked.
async function deleteWideMark(): Promise<CompanyProfile> {
  const { data, error } = await api.DELETE("/company/logo");
  if (error) {
    throwProblem(error);
  }
  return data;
}

async function deleteIconMark(): Promise<CompanyProfile> {
  const { data, error } = await api.DELETE("/company/logo/icon");
  if (error) {
    throwProblem(error);
  }
  return data;
}

type MarkWrites = ReturnType<typeof useMarkWrites>;

/**
 * The two writes one slot needs. Both slots take exactly the same shape — send
 * a file, or take the mark off — and differ only in which endpoint they reach,
 * so a second copy would be a second set of error and settle paths to keep in
 * step with the first.
 */
function useMarkWrites(
  uploadPath: string,
  removeMark: () => Promise<CompanyProfile>,
  settle: (next: CompanyProfile) => void,
) {
  const upload = useMutation({
    mutationFn: async (file: File): Promise<CompanyProfile> => {
      // Sent as multipart by hand rather than through the typed client: the
      // generated client serializes JSON bodies, and this endpoint takes a
      // file part.
      const body = new FormData();
      body.append("file", file);
      // contract-fetch:allow multipart — see the note above
      const response = await fetch(uploadPath, {
        method: "POST",
        body,
        credentials: "include",
      });
      const payload = await response.json().catch(() => undefined);
      if (!response.ok) {
        throwProblem(payload);
      }
      return payload as CompanyProfile;
    },
    onSuccess: settle,
  });
  // Removal carries no body, so it goes through the typed client like every
  // other write. Only the upload has to be spelled by hand.
  const remove = useMutation({ mutationFn: removeMark, onSuccess: settle });
  return { upload, remove };
}

/**
 * One slot: the mark it holds now, and the verbs that change it.
 *
 * `verbs` carries the accessible name of each button rather than its visible
 * word. Two slots side by side would otherwise put two buttons called "Replace"
 * on the page, which a reader walking a list of controls cannot tell apart —
 * and the visible word stays inside the name, so what is said and what is seen
 * still match.
 */
function MarkSlot({
  profile,
  canEdit,
  src,
  square,
  name,
  status,
  hint,
  emptyLabel,
  verbs,
  writes,
}: Readonly<{
  profile: CompanyProfile;
  canEdit: boolean;
  src?: string | null;
  square?: boolean;
  name: string;
  status: string;
  hint: string;
  emptyLabel: string;
  verbs: { add: string; replace: string; remove: string };
  writes: MarkWrites;
}>) {
  const t = useT();
  // The section is named by the heading it already shows, not by a second copy
  // of that text in an `aria-label`: two elements carrying the same accessible
  // name is what makes "the Square icon control" ambiguous to anything that
  // looks one up, a test included.
  const headingId = useId();
  const { upload, remove } = writes;
  // The picker is closed until it is asked for. A dropzone standing open under
  // a mark the company already has reads as the thing to do, and the thing to
  // do on this row is usually nothing.
  const [picking, setPicking] = useState(false);
  // Each verb is disabled by the OTHER one's flight and PENDING on its own: a
  // button that disables itself the moment it is pressed takes the focus with
  // it, where `pending` keeps the focus, marks the wait, and swallows a repeat
  // press — so a double press or a held Enter cannot start a second request
  // whose answer would race the first, nor open a second picker whose file
  // would then be dropped.
  const failure = upload.error ?? remove.error;
  const removeMark = () => {
    // A picker left open under a removal is a second writer: a file dropped
    // while the DELETE is out lands in whichever order the two answer.
    setPicking(false);
    remove.mutate();
  };
  const uploadMark = (file: File) => {
    setPicking(false);
    upload.mutate(file);
  };

  return (
    <section className="company-mark-slot" aria-labelledby={headingId}>
      <b className="t-caption" id={headingId}>
        {name}
      </b>
      <p className="t-caption">{status}</p>
      {!picking && <p className="t-caption">{hint}</p>}
      <MarkPreview profile={profile} src={src} square={square} />
      {canEdit && (
        <div className="company-mark-actions">
          <Button
            small
            aria-label={src ? verbs.replace : verbs.add}
            onClick={() => setPicking((open) => !open)}
            disabled={remove.isPending}
            pending={upload.isPending}
          >
            {src
              ? t("settings.companyMarkReplace")
              : t("settings.companyMarkAdd")}
          </Button>
          {src && (
            <Button
              small
              variant="ghost"
              aria-label={verbs.remove}
              onClick={removeMark}
              disabled={upload.isPending}
              pending={remove.isPending}
            >
              {t("settings.companyMarkRemove")}
            </Button>
          )}
        </div>
      )}
      {picking && canEdit && (
        <FileDropzone
          label={name}
          hint={hint}
          emptyLabel={emptyLabel}
          accept={ACCEPTED_IMAGES}
          onPick={uploadMark}
        />
      )}
      {failure && (
        <Callout tone="danger" live="alert">
          {problemMessageOf(failure, t)}
        </Callout>
      )}
    </section>
  );
}

// The stored image over the record's own monogram, which is the floor under
// both slots: a company that has chosen no picture has a face rather than a gap.
function MarkPreview({
  profile,
  src,
  square,
}: Readonly<{
  profile: CompanyProfile;
  src?: string | null;
  square?: boolean;
}>) {
  const monogram = (
    <Avatar
      identity={profile.organization_id}
      name={profile.display_name}
      shape="organization"
      size="xl"
    />
  );
  const shape = square ? " company-logo-preview-square" : "";
  if (!src) {
    return (
      <div
        className={`company-logo-preview company-logo-preview-empty${shape}`}
      >
        {monogram}
      </div>
    );
  }
  return (
    <div className={`company-logo-preview${shape}`}>
      <CompanyLogo name={profile.display_name} src={src} fallback={monogram} />
    </div>
  );
}
