import { useId, useState } from "react";
import { Button, Field, Modal } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemCodeOf } from "../lib/problem";
import { CompanyDealPicker } from "./companydealpicker";
import {
  type DealSuggestion,
  type DismissDealSuggestionBody,
  useAcceptDealSuggestion,
  useDismissDealSuggestion,
} from "./dealsuggestions.queries";

const reasons = [
  "not_a_deal",
  "already_tracked",
  "wrong_company",
  "historical",
  "other",
] as const;
type Reason = NonNullable<DismissDealSuggestionBody["reason"]>;

export function DealSuggestionAnswerDialog({
  suggestion,
  action,
  onClose,
}: Readonly<{
  suggestion: DealSuggestion;
  action: "link" | "dismiss";
  onClose: () => void;
}>) {
  const t = useT();
  const toast = useToast();
  const heading = useId();
  const [key] = useState(() => crypto.randomUUID());
  const [deal, setDeal] = useState("");
  const [reason, setReason] = useState<Reason>("not_a_deal");
  const accept = useAcceptDealSuggestion();
  const dismiss = useDismissDealSuggestion();
  const pending = accept.isPending || dismiss.isPending;
  const complete = (partial = false) => {
    toast.show(
      t(
        partial
          ? "dealSuggestion.linkedUnlinked"
          : action === "link"
            ? "dealSuggestion.linked"
            : "dealSuggestion.dismissed",
      ),
    );
    onClose();
  };
  const submit = () => {
    if (action === "link")
      accept.mutate(
        {
          id: suggestion.id,
          idempotencyKey: key,
          body: { existing_deal_id: deal },
        },
        { onSuccess: (out) => complete(out.unlinked_activity_ids.length > 0) },
      );
    else
      dismiss.mutate(
        { id: suggestion.id, idempotencyKey: key, body: { reason } },
        { onSuccess: () => complete() },
      );
  };
  return (
    <Modal open onClose={onClose} labelledBy={heading} intent="form">
      <Heading size="large" id={heading} className="modal-title">
        {t(
          action === "link"
            ? "dealSuggestion.linkExisting"
            : "dealSuggestion.dismissTitle",
        )}
      </Heading>
      <p className="sub">
        {t(
          action === "link"
            ? "dealSuggestion.linkEffect"
            : "dealSuggestion.dismissEffect",
          { company: suggestion.company_name },
        )}
      </p>
      <div className="form-stack">
        {action === "link" ? (
          <CompanyDealPicker
            companyId={suggestion.company_id}
            value={deal}
            onChange={setDeal}
          />
        ) : (
          <Field label={t("dealSuggestion.dismissReason")}>
            {(control) => (
              <Select
                {...control}
                value={reason}
                options={reasons.map((value) => ({
                  value,
                  label: t(`dealSuggestion.dismissReason.${value}`),
                }))}
                onChange={(value) => {
                  const found = reasons.find(
                    (candidate) => candidate === value,
                  );
                  if (found) setReason(found);
                }}
              />
            )}
          </Field>
        )}
        <ErrorLine error={accept.error} />
        {dismiss.error && (
          <ErrorLine>
            {t(
              problemCodeOf(dismiss.error) === "conflict"
                ? "dealSuggestion.decided"
                : "dealSuggestion.failed",
            )}
          </ErrorLine>
        )}
      </div>
      <div className="actions">
        <Button onClick={onClose} disabled={pending}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          pending={pending}
          disabled={action === "link" && !deal}
          onClick={submit}
        >
          {t(
            action === "link"
              ? "dealSuggestion.linkExisting"
              : "dealSuggestion.dismiss",
          )}
        </Button>
      </div>
    </Modal>
  );
}
