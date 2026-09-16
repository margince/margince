// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { isOption } from "../app/options";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";

// RECORDING a decision about a domain: the form, what it refuses to send, and
// the write itself.
//
// Separate from the list beside it because it answers a different question. The
// list says where every domain stands, including the ones nobody has judged;
// this is the one place somebody SETTLES one, and settling owes a sentence
// a reader can review later. The two share the row type and its labels and
// nothing else.

export type BlockedDomain = components["schemas"]["BlockedDomain"];

// Every standing a row can be read in, decisions and the open question alike.
// Keyed on the contract's own union so a state added there fails to typecheck
// here rather than rendering as a blank cell.
export const ADMISSION_LABEL: Record<BlockedDomain["admission"], MessageKey> = {
  suppressed: "blockedDomains.admission.suppressed",
  admitted: "blockedDomains.admission.admitted",
  undecided: "blockedDomains.admission.undecided",
};

// The two decisions an entry can carry, as ONE list: the type is derived from
// it and the Select's options are built from it, so the offered choices, their
// labels and the runtime narrowing cannot drift apart (the shape
// consumer-mail-domains.tsx uses for the same reason).
//
// `undecided` is deliberately NOT here. It is a state a row can be READ in and
// never one somebody can choose: the dialog writes decisions, and "nobody has
// decided" is not one of them. Offering it would put a third option in the
// Select that the server's own request schema refuses.
const ADMISSIONS = ["suppressed", "admitted"] as const;
type Admission = (typeof ADMISSIONS)[number];

// The contract's own ceiling on `reason` (SetBlockedDomainRequest.maxLength).
// Held on the control so the reader stops at the limit rather than typing past
// it and losing the sentence to a 422 — the server enforces it either way.
const REASON_MAX = 500;

/** One standing decision, as the dialog receives it before anybody types. */
export type Decision = Readonly<{
  domain: string;
  admission: Admission;
  reason: string;
}>;

export const BLANK_DECISION: Decision = {
  domain: "",
  admission: "suppressed",
  reason: "",
};

function useSetBlockedDomain() {
  const queryClient = useQueryClient();
  return useMutation({
    // Every input arrives as a variable rather than through a closure: the
    // handler belongs to the committed render, so what it passes cannot be
    // older than the control the operator pressed.
    mutationFn: async (decision: Decision) => {
      const { data, error } = await api.PUT("/capture/blocked-domains", {
        body: decision,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["blocked-domains"] });
    },
  });
}

/** Derived from the hook rather than restated, so the two cannot drift. */
export type SetDecision = ReturnType<typeof useSetBlockedDomain>;

export { useSetBlockedDomain };

/**
 * The write, in a dialog.
 *
 * It is only ever opened by a seat holding `company:update` — both verbs
 * that open it are refused without the grant — so nothing in here restates the
 * denial. The mutation belongs to the CARD rather than to this component,
 * because what landed is reported after the dialog has closed.
 */
export function DecisionDialog({
  initial,
  set,
  onClose,
}: Readonly<{ initial: Decision; set: SetDecision; onClose: () => void }>) {
  const t = useT();
  const headingId = useId();
  const [domain, setDomain] = useState(initial.domain);
  const [admission, setAdmission] = useState<Admission>(initial.admission);
  const [reason, setReason] = useState(initial.reason);
  const trimmedDomain = domain.trim();
  const trimmedReason = reason.trim();
  const ready = trimmedDomain !== "" && trimmedReason !== "";
  return (
    <Modal open onClose={onClose} labelledBy={headingId}>
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("blockedDomains.record")}
      </Heading>
      <form
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (!ready) {
            return;
          }
          set.mutate(
            {
              domain: trimmedDomain,
              admission,
              reason: trimmedReason,
            },
            { onSuccess: onClose },
          );
        }}
      >
        <div className="form-row">
          <Field label={t("blockedDomains.domainLabel")} required>
            {(control) => (
              <TextInput
                {...control}
                data-testid="blocked-domain-input"
                placeholder={t("blockedDomains.domainPlaceholder")}
                value={domain}
                onChange={(event) => setDomain(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("blockedDomains.admissionLabel")}>
            {(control) => (
              <Select
                {...control}
                value={admission}
                onChange={(value) => {
                  if (isOption(value, ADMISSIONS)) {
                    setAdmission(value);
                  }
                }}
                options={ADMISSIONS.map((value) => ({
                  value,
                  label: t(ADMISSION_LABEL[value]),
                }))}
              />
            )}
          </Field>
        </div>
        <Field
          label={t("blockedDomains.reasonLabel")}
          hint={t("blockedDomains.reasonHint")}
          required
        >
          {(control) => (
            <TextInput
              {...control}
              data-testid="blocked-domain-reason"
              maxLength={REASON_MAX}
              placeholder={t("blockedDomains.reasonPlaceholder")}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          )}
        </Field>
        {set.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("blockedDomains.saveFailed")}
          >
            {problemMessageOf(set.error, t)}
          </Callout>
        )}
        <div className="form-actions">
          <Button small type="button" onClick={onClose}>
            {t("create.cancel")}
          </Button>
          <Button
            small
            type="submit"
            variant="primary"
            disabled={set.isPending || !ready}
          >
            {t("blockedDomains.save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
