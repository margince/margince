// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
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
 * The installation's own mark: what it is now, and the two things a person can
 * do about it.
 *
 * The mark is normally the one the website read resolved from the company's own
 * site. This is the other door — for an installation whose site declares no
 * icon, and for the read that resolved the wrong picture. Uploading takes the
 * field: while a person's own mark stands, a later read leaves it alone.
 * Removing gives it back, so the record returns to its monogram and the next
 * read may resolve one again.
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
  const upload = useMutation({
    mutationFn: async (file: File): Promise<CompanyProfile> => {
      // Sent as multipart by hand rather than through the typed client: the
      // generated client serializes JSON bodies, and this endpoint takes a
      // file part.
      const body = new FormData();
      body.append("file", file);
      // contract-fetch:allow multipart — see the note above
      const response = await fetch("/v1/company/logo", {
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
  const remove = useMutation({
    mutationFn: async (): Promise<CompanyProfile> => {
      const { data, error } = await api.DELETE("/company/logo");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: settle,
  });
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
    <div className="company-mark">
      <div className="company-mark-body">
        <b>{t("settings.companyMark")}</b>
        <p className="t-caption">
          {profile.logo_url
            ? t("settings.companyMarkPresent")
            : t("settings.companyMarkNone")}
        </p>
        {!picking && (
          <p className="t-caption">{t("settings.companyMarkHint")}</p>
        )}
        <div
          className={
            profile.logo_url
              ? "company-logo-preview"
              : "company-logo-preview company-logo-preview-empty"
          }
        >
          {profile.logo_url ? (
            <CompanyLogo
              name={profile.display_name}
              src={profile.logo_url}
              fallback={
                <Avatar
                  identity={profile.organization_id}
                  name={profile.display_name}
                  shape="organization"
                  size="xl"
                />
              }
            />
          ) : (
            <Avatar
              identity={profile.organization_id}
              name={profile.display_name}
              shape="organization"
              size="xl"
            />
          )}
        </div>
        {canEdit && (
          <div className="company-mark-actions">
            <Button
              small
              onClick={() => setPicking((open) => !open)}
              disabled={remove.isPending}
              pending={upload.isPending}
            >
              {profile.logo_url
                ? t("settings.companyMarkReplace")
                : t("settings.companyMarkAdd")}
            </Button>
            {profile.logo_url && (
              <Button
                small
                variant="ghost"
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
            label={t("settings.companyMarkPick")}
            hint={t("settings.companyMarkHint")}
            emptyLabel={t("settings.companyMarkEmpty")}
            accept={ACCEPTED_IMAGES}
            onPick={uploadMark}
          />
        )}
        {failure && (
          <Callout tone="danger" live="alert">
            {problemMessageOf(failure, t)}
          </Callout>
        )}
      </div>
    </div>
  );
}
