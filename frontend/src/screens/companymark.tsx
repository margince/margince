// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useRef } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Avatar, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CompanyLogo } from "../design-system/companylogo";
import { FileDropzoneControl } from "../design-system/filedropzone";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { postMultipart } from "../lib/multipart";
import { problemMessageOf, unwrap } from "../lib/problem";
import { storeCompany } from "./installationcompany";
import "./companymark.css";

type CompanyProfile = components["schemas"]["CompanyProfile"];

// What the picker offers, and what the server will decode. Both lists say the
// same thing, and this one is the browser's filter only: a drop is not filtered
// by it, and the server refuses what it cannot read either way.
const ACCEPTED_IMAGES =
  "image/png,image/jpeg,image/gif,image/webp,image/svg+xml,image/x-icon";

/**
 * The installation's own marks: what they are now, and what a contact can do
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
 * takes the field: while a contact's own mark stands, a later read leaves it
 * alone. Removing gives it back, so the record returns to its monogram and the
 * next read may resolve one again. Nothing but an upload ever fills the square
 * slot, so it has no read to hold off.
 */
export function CompanyMark({
  profile,
  canEdit,
  readOnly = false,
}: Readonly<{
  profile: CompanyProfile;
  canEdit: boolean;
  readOnly?: boolean;
}>) {
  const t = useT();
  const client = useQueryClient();
  // The response IS the new profile, so it is written straight into the entry
  // this card reads, and /me is refreshed so the rail wears the new mark.
  const settle = (next: CompanyProfile) => {
    storeCompany(client, next);
  };
  const wide = useMarkWrites("/v1/company/logo", deleteWideMark, settle);
  const icon = useMarkWrites("/v1/company/logo/icon", deleteIconMark, settle);
  const hintId = useId();

  return (
    <Panel title={t("settings.brandTitle")}>
      <PanelBody>
        {readOnly && <p>{t("settings.companyReadOnly")}</p>}
        <div className="brand-slots">
          <MarkSlot
            profile={profile}
            canEdit={canEdit}
            src={profile.logo_url}
            shape="wide"
            name={t("settings.companyMarkWide")}
            hintId={hintId}
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
            shape="square"
            name={t("settings.companyMarkIcon")}
            hintId={hintId}
            emptyLabel={t("settings.companyMarkIconEmpty")}
            verbs={{
              add: t("settings.companyMarkAddIcon"),
              replace: t("settings.companyMarkReplaceIcon"),
              remove: t("settings.companyMarkRemoveIcon"),
            }}
            writes={icon}
          />
        </div>
        <p className="t-caption" id={hintId}>
          {t("settings.brandHint")}
        </p>
      </PanelBody>
    </Panel>
  );
}

// The removals, spelled at module scope because the typed client keys on the
// path LITERAL: a variable would leave the response `unknown` and the profile
// this write settles unchecked.
async function deleteWideMark(): Promise<CompanyProfile> {
  return unwrap(await api.DELETE("/company/logo"));
}

async function deleteIconMark(): Promise<CompanyProfile> {
  return unwrap(await api.DELETE("/company/logo/icon"));
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
      const payload = await postMultipart(uploadPath, { file });
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
 * One slot: a tile in the mark's shape that previews it and takes a file, then
 * the slot's name and its verbs. `verbs` holds each button's accessible name,
 * so two slots never offer two controls both called "Replace".
 */
function MarkSlot({
  profile,
  canEdit,
  src,
  shape,
  name,
  hintId,
  emptyLabel,
  verbs,
  writes,
}: Readonly<{
  profile: CompanyProfile;
  canEdit: boolean;
  src?: string | null;
  shape: "wide" | "square";
  name: string;
  hintId: string;
  emptyLabel: string;
  verbs: { add: string; replace: string; remove: string };
  writes: MarkWrites;
}>) {
  const t = useT();
  const nameId = useId();
  const inputId = useId();
  const tile = useRef<HTMLDivElement>(null);
  const { upload, remove } = writes;
  // The tile takes nothing while a write is out: a drop racing a DELETE lands
  // in whichever order the two answer.
  const busy = upload.isPending || remove.isPending;
  const control = {
    id: inputId,
    "aria-labelledby": nameId,
    "aria-describedby": hintId,
    disabled: busy,
  };
  const choose = () => {
    tile.current?.querySelector<HTMLInputElement>("input[type=file]")?.click();
  };

  return (
    <section
      className={`brand-slot brand-slot-${shape}`}
      aria-labelledby={nameId}
    >
      <div
        ref={tile}
        className={src ? "brand-tile is-filled" : "brand-tile"}
        aria-busy={busy || undefined}
      >
        {canEdit && (
          <FileDropzoneControl
            control={control}
            emptyLabel={emptyLabel}
            accept={ACCEPTED_IMAGES}
            onPick={(file) => upload.mutate(file)}
          />
        )}
        <div className="brand-tile-mark">
          <MarkPreview profile={profile} src={src} />
        </div>
      </div>
      <span className="t-name" id={nameId}>
        {name}
      </span>
      {canEdit && (
        <div className="brand-slot-verbs">
          {/* Each verb is disabled by the OTHER one's flight and pending on its
              own, so focus stays on a pressed verb and a repeat press is
              swallowed rather than racing the first request. */}
          <Button
            variant="ghost"
            aria-label={src ? verbs.replace : verbs.add}
            onClick={choose}
            disabled={remove.isPending}
            pending={upload.isPending}
          >
            {src
              ? t("settings.companyMarkReplace")
              : t("settings.companyMarkAdd")}
          </Button>
          {src && (
            <Button
              variant="ghost"
              aria-label={verbs.remove}
              onClick={() => remove.mutate()}
              disabled={upload.isPending}
              pending={remove.isPending}
            >
              {t("settings.companyMarkRemove")}
            </Button>
          )}
        </div>
      )}
      {/* One notice per verb: a single heading over `upload.error ?? remove.error`
          left the reader unable to tell which of the two the server refused. */}
      {upload.error !== null && (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("settings.companyMarkUploadFailed")}
        >
          {problemMessageOf(upload.error, t)}
        </Callout>
      )}
      {remove.error !== null && (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("settings.companyMarkRemoveFailed")}
        >
          {problemMessageOf(remove.error, t)}
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
}: Readonly<{ profile: CompanyProfile; src?: string | null }>) {
  const monogram = (
    <Avatar
      identity={profile.company_id}
      name={profile.display_name}
      size="md"
    />
  );
  if (!src) {
    return monogram;
  }
  return (
    <CompanyLogo name={profile.display_name} src={src} fallback={monogram} />
  );
}
