// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Avatar } from "../../design-system/atoms";
import { CompanyLogo } from "../../design-system/companylogo";
import { ErrorLine } from "../../design-system/errorline";
import { ImageSlot } from "../../design-system/imageslot";
import { useT } from "../../i18n";
import type { LegalEntity, LogoWrite } from "../../lib/legalentities.queries";
import { problemMessageOf } from "../../lib/problem";

const ACCEPTED_IMAGES =
  "image/png,image/jpeg,image/gif,image/webp,image/svg+xml,image/x-icon";

// The logo is stored the moment it is chosen, as on the company profile. A
// file is no text the form could hold back until Save.
export function EntityLogo({
  entity,
  write,
  saving,
  version,
  onVersion,
}: Readonly<{
  entity: LegalEntity;
  write: LogoWrite;
  saving: boolean;
  version: number;
  onVersion: (version: number) => void;
}>) {
  const t = useT();
  const hintId = useId();
  const [src, setSrc] = useState(entity.logo_url ?? null);
  const removing = write.isPending && write.variables?.file === null;
  const send = (file: File | null) =>
    write.mutate(
      { id: entity.id, version, file },
      {
        onSuccess: (written) => {
          if (written === null) return;
          onVersion(written.version);
          setSrc(written.logoUrl);
        },
      },
    );
  const monogram = <Avatar identity={entity.id} name={entity.display_name} />;
  return (
    <>
      <ImageSlot
        shape="wide"
        filled={Boolean(src)}
        preview={
          src ? (
            <CompanyLogo
              name={entity.display_name}
              src={src}
              fallback={monogram}
            />
          ) : (
            monogram
          )
        }
        editable
        name={t("legalEntities.sectionLogo")}
        describedBy={hintId}
        emptyLabel={t("legalEntities.logoEmpty")}
        accept={ACCEPTED_IMAGES}
        verbs={{
          edit: t("settings.imageEdit"),
          editLabel: t("legalEntities.logoEdit"),
          upload: t("legalEntities.logoAdd"),
          remove: t("legalEntities.logoRemove"),
        }}
        // The form's own save moves the version this write needs, so the
        // slot waits on it as on its own write.
        uploading={(write.isPending && !removing) || saving}
        removing={removing}
        onPick={send}
        onRemove={() => send(null)}
      />
      <p className="t-caption" id={hintId}>
        {t("legalEntities.logoHint")}
      </p>
      <ErrorLine error={write.isError && problemMessageOf(write.error, t)} />
    </>
  );
}
