// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { Button, Field, Textarea } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { useFieldFaults } from "../../design-system/savepage";
import { useLocale, useT } from "../../i18n";
import { blankRequired, unwrap } from "../../lib/problem";
import type { SalesDecision } from "./analytics.decisions";
import { DECISIONS_KEY } from "./analytics.decisions.key";

/** Record outcome: what work was completed. The task closes; the deal stays. */
export function OutcomeDialog({
  decision,
  onClose,
}: Readonly<{ decision: SalesDecision; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const titleId = useId();
  const formId = useId();
  const [faults, faultsRoot] = useFieldFaults();
  const client = useQueryClient();
  const [result, setResult] = useState("");
  const resultLabel = t("analytics.decisions.whatWasDone");
  const close = useMutation({
    mutationFn: async (text: string) =>
      unwrap(
        await api.POST("/analytics/decisions/{id}/outcome", {
          params: { path: { id: decision.id } },
          body: { result: text },
        }),
      ),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: [DECISIONS_KEY] });
      onClose();
    },
  });
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {t("analytics.decisions.recordOutcome")}
      </Heading>
      <p className="t-caption">{decision.decision}</p>
      <form
        id={formId}
        ref={faultsRoot}
        noValidate
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          const blank = blankRequired(locale, {
            // The label is a question, so the sentence leaves it out.
            result: [undefined, result],
          });
          if (!faults.show(blank)) close.mutate(result.trim());
        }}
      >
        <Field label={resultLabel} required error={faults.of("result")}>
          {(field) => (
            <Textarea
              {...field}
              value={result}
              rows={4}
              onChange={(event) => {
                setResult(event.target.value);
                faults.edited("result");
              }}
            />
          )}
        </Field>
        <p className="t-caption">{t("analytics.decisions.outcomeNote")}</p>
        <ErrorLine error={close.error} />
      </form>
      <div className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("reporting.cancel")}
        </Button>
        <Button type="submit" form={formId} pending={close.isPending}>
          {t("analytics.decisions.saveOutcome")}
        </Button>
      </div>
    </Modal>
  );
}
