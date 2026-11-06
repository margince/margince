import { accessLabel, type GrantDraft } from "../../components/share.parts";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useT } from "../../i18n";

export function ShareConfirmation({
  draft,
  bulk,
  open,
  onClose,
  returnFocusTo,
  onConfirm,
  pending,
  error,
}: Readonly<{
  draft: GrantDraft | null;
  bulk: boolean;
  open: boolean;
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
  onConfirm: (draft: GrantDraft) => void;
  pending: boolean;
  error: string | null;
}>) {
  const t = useT();
  if (draft === null) return null;
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t(bulk ? "share.bulkConfirmTitle" : "share.downgradeTitle")}
      confirmLabel={
        bulk
          ? t("share.grant")
          : t("share.downgradeConfirm", { to: accessLabel(draft.access, t) })
      }
      confirmVariant={bulk ? "primary" : "danger"}
      returnFocusTo={returnFocusTo}
      onConfirm={() => onConfirm(draft)}
      pending={pending}
      error={error}
    >
      <p data-testid="share-downgrade-body">
        {bulk
          ? t("share.bulkConfirmBody", {
              name: draft.subject.name,
              access: accessLabel(draft.access, t),
            })
          : draft.change !== "new" &&
            t("share.downgradeBody", {
              name: draft.subject.name,
              from: accessLabel(draft.heldAccess, t),
              to: accessLabel(draft.access, t),
            })}
      </p>
    </ConfirmModal>
  );
}
