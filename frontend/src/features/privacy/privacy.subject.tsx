// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { Download } from "lucide-react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  ContactCaseEntry,
  otherCaseMembers,
} from "../../components/contactcase.entry";
import { useContactCase } from "../../components/contactcase.queries";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { downloadBytes, filenameFromDisposition } from "../../lib/download";
import { problemMessageOf, throwProblem } from "../../lib/problem";
import { isUuid } from "../../lib/uuid";

type DataSubjectRequest = components["schemas"]["DataSubjectRequest"];

// Whether the request names a record rather than free text. The label alone
// cannot say: it is null for a record the reader cannot see, archived ones
// included, which the server still answers for.
export function namesRecord(dsr: DataSubjectRequest): boolean {
  return isUuid(dsr.subject_ref);
}

// The duplicate hint shows on a visible contact. The package shows on an
// access request that names a record not known to be a lead. The server
// answers for any other record it cannot export.
export function DsrSubjectActions({
  dsr,
  canWork,
}: Readonly<{ dsr: DataSubjectRequest; canWork: boolean }>) {
  if (!namesRecord(dsr)) return null;
  return (
    <>
      {dsr.subject_kind === "contact" && (
        <DsrPossibleDuplicates contactId={dsr.subject_ref} />
      )}
      {canWork && dsr.kind === "access" && dsr.subject_kind !== "lead" && (
        <DsrPackageDownload requestId={dsr.id} />
      )}
    </>
  );
}

// The case read only shows pairs the reader may see, so the line says "may".
function DsrPossibleDuplicates({ contactId }: Readonly<{ contactId: string }>) {
  const t = useT();
  if (otherCaseMembers(useContactCase(contactId).data) < 1) return null;
  return (
    <div className="form-stack">
      <p>{t("privacy.possibleDuplicates")}</p>
      <div>
        <ContactCaseEntry contactId={contactId} />
      </div>
    </div>
  );
}

// The server is the gate (a human admin with unbounded row scope); a refusal
// shows its own reason. Downloading leaves the request's status as it was.
function DsrPackageDownload({ requestId }: Readonly<{ requestId: string }>) {
  const t = useT();
  const download = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await api.GET(
        "/data-subject-requests/{id}/package",
        { params: { path: { id: requestId } }, parseAs: "blob" },
      );
      if (error) throwProblem(error);
      return {
        bytes: data,
        filename: filenameFromDisposition(
          response.headers.get("content-disposition"),
          `data-subject-request-${requestId}.json`,
        ),
      };
    },
    onSuccess: ({ bytes, filename }) =>
      downloadBytes(bytes, filename, "application/json"),
  });
  return (
    <div>
      <Button pending={download.isPending} onClick={() => download.mutate()}>
        <Download aria-hidden />
        {t("privacy.downloadPackage")}
      </Button>
      {download.isError && (
        <ErrorLine>{problemMessageOf(download.error, t)}</ErrorLine>
      )}
    </div>
  );
}
