// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useT } from "../../i18n";
import { isVersionSkewOf, problemMessageOf, unwrap } from "../../lib/problem";

export function EndPartnerProgramAction({
  companyId,
  version,
  refusal,
  onEnded,
}: Readonly<{
  companyId: string;
  version: number | undefined;
  refusal?: string;
  onEnded: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [readVersion, setReadVersion] = useState(version);
  const mutation = useMutation({
    mutationFn: async ({
      id,
      version,
    }: {
      id: string;
      version: number | undefined;
    }) => {
      unwrap(
        await api.DELETE("/companies/{id}/partner", {
          params: { path: { id }, ...ifMatch(requireVersion(version)) },
        }),
      );
    },
    onError: (error) => {
      if (isVersionSkewOf(error))
        queryClient.invalidateQueries({ queryKey: ["partner", companyId] });
    },
  });
  return (
    <>
      <Button
        variant="danger"
        reason={refusal}
        onClick={() => {
          setReadVersion(version);
          mutation.reset();
          setOpen(true);
        }}
      >
        {t("partner.endProgram")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("partner.endProgram")}
        confirmLabel={t("partner.endProgram")}
        confirmVariant="danger"
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
        onConfirm={() =>
          mutation
            .mutateAsync({ id: companyId, version: readVersion })
            .then(() => {
              setOpen(false);
              onEnded();
            })
            .catch(() => undefined)
        }
      >
        <p>{t("partner.endProgramHelp")}</p>
      </ConfirmModal>
    </>
  );
}
