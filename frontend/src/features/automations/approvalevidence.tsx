import { SignatureHtml } from "../../components/signaturehtml";
import type { DecisionApproval } from "../../design-system/decisioncard";
import { EmailAttachments } from "../../design-system/emailmessage";
import { EmailText } from "../../design-system/emailtext";
import { SourceEvidence } from "../../design-system/sourceevidence";
import { SurfaceState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { useSourceOpener } from "../../lib/opensource";

export function ApprovalEvidence({
  approval,
}: Readonly<{ approval: DecisionApproval }>) {
  return (
    <>
      <ApprovalMessage approval={approval} />
      <ApprovalSources approval={approval} />
    </>
  );
}

function ApprovalMessage({
  approval,
}: Readonly<{ approval: DecisionApproval }>) {
  const t = useT();
  const preview = approval.preview;
  if (preview?.version !== 2 || preview.state !== "ready") return null;
  const body = preview.details?.find((detail) => detail.field === "body")?.value
    .text;
  const html = preview.details?.find((detail) => detail.field === "html_body")
    ?.value.text;
  const signature = preview.details?.find(
    (detail) => detail.field === "signature_text",
  )?.value.text;
  const signatureHtml = preview.details?.find(
    (detail) => detail.field === "signature_html",
  )?.value.text;
  return (
    <>
      {body && <EmailText body={body} />}
      {html && (
        <SignatureHtml html={html} title={t("approval.preview.htmlBody")} />
      )}
      {signatureHtml ? (
        <SignatureHtml
          html={signatureHtml}
          title={t("approval.preview.senderSignature")}
        />
      ) : signature ? (
        <EmailText body={signature} />
      ) : null}
      <EmailAttachments files={preview.files ?? []} />
    </>
  );
}

function ApprovalSources({
  approval,
}: Readonly<{ approval: DecisionApproval }>) {
  const t = useT();
  const openerFor = useSourceOpener();
  const legacySource = approval.proposed_change?.source_activity_id;
  const source =
    approval.target_entity_type === "activity" && approval.target_entity_id
      ? approval.target_entity_id
      : typeof legacySource === "string"
        ? legacySource
        : null;
  const sources = approval.preview?.evidence
    ? [
        ...new Set(
          approval.preview.evidence.flatMap((item) =>
            item.source_type === "activity" && item.source_id
              ? [item.source_id]
              : [],
          ),
        ),
      ]
    : approval.kind === "commitment_task" && source
      ? [source]
      : [];
  if (sources.length === 0 && approval.kind !== "commitment_task") return null;
  if (sources.length === 0) {
    return (
      <SurfaceState
        label={t("stat.evidence")}
        state="empty"
        emptyLabel={t("trust.sourceUnknown")}
        loadingLabel={t("tasks.detailLoading")}
      >
        {null}
      </SurfaceState>
    );
  }
  return (
    <>
      {sources.map((id) => (
        <SourceEvidence key={id} activityId={id} openerFor={openerFor} />
      ))}
    </>
  );
}
