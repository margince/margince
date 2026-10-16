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
import type { MessageKey } from "../i18n/en";
import { postMultipart } from "../lib/multipart";
import {
  ProblemError,
  problemCode,
  problemMessageOf,
  unwrap,
} from "../lib/problem";
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
}: Readonly<{
  profile: CompanyProfile;
  canEdit: boolean;
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
  const wideRefusalId = useId();
  const iconRefusalId = useId();

  return (
    <Panel title={t("settings.brandTitle")}>
      <PanelBody>
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
            refusalId={wide.refusal ? wideRefusalId : undefined}
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
            refusalId={icon.refusal ? iconRefusalId : undefined}
          />
        </div>
        {(wide.refusal || icon.refusal) && (
          <div className="brand-refusals">
            {wide.refusal && (
              <MarkRefusal
                id={wideRefusalId}
                refusal={wide.refusal}
                titles={{
                  upload: t("settings.companyMarkUploadFailedWide"),
                  remove: t("settings.companyMarkRemoveFailedWide"),
                }}
              />
            )}
            {icon.refusal && (
              <MarkRefusal
                id={iconRefusalId}
                refusal={icon.refusal}
                titles={{
                  upload: t("settings.companyMarkUploadFailedIcon"),
                  remove: t("settings.companyMarkRemoveFailedIcon"),
                }}
              />
            )}
          </div>
        )}
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
  // One mutation for both verbs, so a slot has one error and a new attempt of
  // either verb clears the last refusal. `null` is the removal.
  const write = useMutation({
    mutationFn: async (file: File | null): Promise<CompanyProfile> => {
      if (file === null) {
        return removeMark();
      }
      const payload = await postMultipart(uploadPath, { file });
      return payload as CompanyProfile;
    },
    onSuccess: settle,
  });
  const verb = write.variables === null ? "remove" : "upload";
  return {
    send: (file: File | null) => write.mutate(file),
    uploading: write.isPending && verb === "upload",
    removing: write.isPending && verb === "remove",
    refusal: write.error === null ? null : { verb, error: write.error },
  } as const;
}

type MarkRefusalState = NonNullable<MarkWrites["refusal"]>;

// The server's sentence is for an API reader; these codes get the reader's language.
const REFUSAL_COPY = new Map<string, MessageKey>([
  ["unsupported_media_type", "settings.companyMarkUnsupported"],
  ["body_too_large", "settings.companyMarkTooLarge"],
  ["svg_text_not_outlined", "settings.companyMarkSvgText"],
]);

// Below the tiles, not inside a slot: a notice in a slot widens it and pushes
// the other tile onto its own row.
function MarkRefusal({
  id,
  refusal,
  titles,
}: Readonly<{
  id: string;
  refusal: MarkRefusalState;
  titles: Record<MarkRefusalState["verb"], string>;
}>) {
  const t = useT();
  const copy = REFUSAL_COPY.get(
    problemCode(
      refusal.error instanceof ProblemError ? refusal.error.problem : null,
    ) ?? "",
  );
  return (
    <div id={id}>
      <Callout tone="danger" kind="outcome" title={titles[refusal.verb]}>
        {copy ? t(copy) : problemMessageOf(refusal.error, t)}
      </Callout>
    </div>
  );
}

// `verbs` holds each button's accessible name, so two slots never offer two
// controls both called "Replace".
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
  refusalId,
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
  refusalId?: string;
}>) {
  const t = useT();
  const nameId = useId();
  const inputId = useId();
  const tile = useRef<HTMLDivElement>(null);
  const { send, uploading, removing } = writes;
  const describedBy = refusalId ? `${hintId} ${refusalId}` : hintId;
  // The tile takes nothing while a write is out: a drop racing a DELETE lands
  // in whichever order the two answer.
  const busy = uploading || removing;
  const control = {
    id: inputId,
    "aria-labelledby": nameId,
    "aria-describedby": describedBy,
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
            onPick={send}
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
            aria-describedby={refusalId}
            disabled={removing}
            pending={uploading}
          >
            {src
              ? t("settings.companyMarkReplace")
              : t("settings.companyMarkAdd")}
          </Button>
          {src && (
            <Button
              variant="ghost"
              aria-label={verbs.remove}
              onClick={() => send(null)}
              aria-describedby={refusalId}
              disabled={uploading}
              pending={removing}
            >
              {t("settings.companyMarkRemove")}
            </Button>
          )}
        </div>
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
