import { useMutation } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ChoiceList } from "../design-system/choicelist";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";

// Sharing a forecast view.
//
// The two kinds make different promises and the dialog says so in words rather
// than in a label: a live link answers what the reader may see TODAY, a frozen
// one answers what was true at a moment. A reader handed a number without
// knowing which of those it is cannot place it, and the frozen one is the case
// where being wrong matters — a figure from three weeks ago, read as current.
type ShareKind = "live" | "snapshot";

// The link, held in state and never re-derivable. The server returns the token
// once; there is nothing to read it back from, which is why the dialog says
// what leaving costs before it lets the reader leave.
type IssuedShare = Readonly<{ token: string; expiresAt: string }>;

export function ShareViewButton({
  target,
  snapshotId,
}: Readonly<{
  target: string;
  // The frozen state a snapshot share would serve. Absent means there is none
  // to freeze yet, and the snapshot choice is unavailable rather than offered
  // and then refused by the server.
  snapshotId?: string;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button small onClick={() => setOpen(true)}>
        {t("analytics.share.open")}
      </Button>
      {open && (
        <ShareDialog
          target={target}
          snapshotId={snapshotId}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

function ShareDialog({
  target,
  snapshotId,
  onClose,
}: Readonly<{
  target: string;
  snapshotId?: string;
  onClose: () => void;
}>) {
  const t = useT();
  const [kind, setKind] = useState<ShareKind>("live");
  const [issued, setIssued] = useState<IssuedShare | null>(null);

  const create = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/forecast/shares", {
        body: {
          kind,
          target,
          // The workspace scope names no subject, which is what the server
          // refuses a scope_id alongside. Stated rather than left to a default
          // so the request says which forecast it shares.
          scope_kind: "workspace" as const,
          ...(kind === "snapshot" && snapshotId
            ? { snapshot_id: snapshotId }
            : {}),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) =>
      setIssued({ token: data.token, expiresAt: data.expires_at }),
  });

  if (issued) {
    return <ShareLinkReveal share={issued} onClose={onClose} />;
  }

  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("analytics.share.title")}
      confirmLabel={t("analytics.share.create")}
      onConfirm={() => create.mutate()}
      pending={create.isPending}
      error={create.error ? problemMessageOf(create.error, t) : undefined}
    >
      <ChoiceList
        legend={t("analytics.share.kindLegend")}
        value={kind}
        onChange={setKind}
        choices={[
          {
            value: "live",
            label: t("analytics.share.liveLabel"),
            description: t("analytics.share.liveHelp"),
          },
          {
            value: "snapshot",
            label: t("analytics.share.snapshotLabel"),
            description: snapshotId
              ? t("analytics.share.snapshotHelp")
              : t("analytics.share.snapshotUnavailable"),
          },
        ]}
      />
      <p className="t-caption">{t("analytics.share.expiryNote")}</p>
    </ConfirmModal>
  );
}

// The link, shown once.
//
// Dismissing destroys the only copy: it lives in this component's state and no
// read returns it. So Copy is the primary act and Done is the quiet one, and
// the caution says in words what leaving costs — the same shape the webhook
// signing secret settled on, for the same reason.
function ShareLinkReveal({
  share,
  onClose,
}: Readonly<{ share: IssuedShare; onClose: () => void }>) {
  const t = useT();
  const headingId = useId();
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const url = shareUrl(share.token);

  async function copyLink() {
    if (!navigator.clipboard) {
      setCopyFailed(true);
      return;
    }
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopied(false);
      setCopyFailed(true);
    }
  }

  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("analytics.share.linkTitle")}
      confirmLabel={
        copied ? t("analytics.share.copied") : t("analytics.share.copy")
      }
      onConfirm={copyLink}
      actionsLead={
        <Button small onClick={onClose}>
          {t("analytics.share.done")}
        </Button>
      }
    >
      <p id={headingId} className="t-caption">
        {t("analytics.share.linkWarning")}
      </p>
      <pre className="code-block t-mono" data-testid="forecast-share-link">
        {url}
      </pre>
      {copyFailed && (
        <Callout tone="danger" live="alert">
          {t("analytics.share.copyFailed")}
        </Callout>
      )}
      {!copied && (
        <p className="t-caption">{t("analytics.share.leaveWarning")}</p>
      )}
    </ConfirmModal>
  );
}

// shareUrl builds the address the recipient opens.
//
// From the running origin rather than a configured base: a link built against a
// base somebody set once stops working the day the installation moves, and it
// fails in the recipient's hands rather than here.
//
// A HASH route, which is what this app serves. A path-shaped address would 404
// on the static server — nothing routes it — and the reader would find that out
// after sending the link on. dealroomaccess.tsx builds the buyer's link the
// same way for the same reason.
function shareUrl(token: string): string {
  return `${window.location.origin}/#/analytics/shared/${token}`;
}
