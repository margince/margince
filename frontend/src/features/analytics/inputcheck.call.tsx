import { createContext, type ReactNode, useContext, useId } from "react";
import type { components } from "../../api/schema";
import { Button, Modal } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useMe } from "../../lib/session";

type InputCheck = components["schemas"]["InputCheck"];

// The call editor a review row may open: the forecast view's own, for the
// population and period it shows. A call is filed for that population and
// period, so the action is offered only for a finding about that same call.
export type CallEditor = {
  open: () => void;
  scope: { scope_kind: string; scope_id?: string };
  periodStart: string;
};

const CallEditorContext = createContext<CallEditor | null>(null);

export function CallEditorProvider({
  editor,
  children,
}: Readonly<{ editor: CallEditor | null; children: ReactNode }>) {
  return <CallEditorContext value={editor}>{children}</CallEditorContext>;
}

// A finding about a manager's call is answered by calling again, and only its
// author may. Everybody else reads what moved and is told who can act.
export function CallCheckSheet({
  check,
  open,
  onClose,
  returnFocusTo,
  closeLabel,
}: Readonly<{
  check: InputCheck;
  open: boolean;
  onClose: () => void;
  returnFocusTo?: () => HTMLElement | null;
  closeLabel: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const headingId = useId();
  const me = useMe();
  const editor = useContext(CallEditorContext);
  const author =
    me.data?.user.id !== undefined && me.data.user.id === check.owner_id;
  const claim = check.claim ?? {};
  const sameCall =
    editor !== null &&
    claim.scope_kind === editor.scope.scope_kind &&
    (claim.scope_id ?? undefined) === editor.scope.scope_id &&
    claim.period_start === editor.periodStart;
  const moved = check.observed?.changed_deals;
  return (
    <Modal
      intent="form"
      open={open}
      onClose={onClose}
      labelledBy={headingId}
      returnFocusTo={returnFocusTo}
    >
      <Heading id={headingId} size="large" className="modal-title">
        {t("review.forecastCallMoved")}
      </Heading>
      <p>
        {check.observed?.comparison === "limited"
          ? t("review.callLimitedHelp")
          : typeof moved === "number"
            ? plural("review.callMovedHelp", moved, {
                count: formatNumber(moved, locale),
              })
            : t("review.callMovedWithheldHelp")}
      </p>
      {author && !sameCall && <p>{t("review.callOpenItsForecast")}</p>}
      {!author && <p>{t("review.callAuthorOnly")}</p>}
      <div className="actions">
        <Button onClick={onClose}>{closeLabel}</Button>
        {author && sameCall && (
          <Button
            variant="primary"
            onClick={() => {
              onClose();
              editor.open();
            }}
          >
            {t("review.submitNewCall")}
          </Button>
        )}
      </div>
    </Modal>
  );
}
