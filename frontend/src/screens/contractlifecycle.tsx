// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useInstallationSettings } from "../app/uploadlimit";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { MoneyInput } from "../design-system/moneyinput";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import { draftProblem, pricedIn } from "./contractform";

// margince#3286: the three transitions a signed agreement actually goes
// through after it is first recorded — renew, assert a status, record a
// cancellation. Store.Renew / ChangeStatus / Cancel and their endpoints
// (POST /contracts/{id}/renewal /status /cancellation) have always been
// correct; nothing in the app could reach them.

type Contract = components["schemas"]["Contract"];
type ContractStatus = NonNullable<Contract["status"]>;
type ValueBasis = NonNullable<
  components["schemas"]["RenewContractRequest"]["value_basis"]
>;

// A status a contract can only ARRIVE at through renewal — the server sets it
// on the predecessor, in the same transaction that creates the successor
// (Store.Renew, contract_lifecycle.go). Asserting it directly is a transition
// refuseInvalidTransition refuses, so it is never offered here.
const ASSERTABLE_STATUSES: ContractStatus[] = [
  "draft",
  "active",
  "expired",
  "cancelled",
];

const STATUS_LABEL_KEY: Record<ContractStatus, MessageKey> = {
  draft: "contracts.status.draft",
  active: "contracts.status.active",
  expired: "contracts.status.expired",
  cancelled: "contracts.status.cancelled",
  superseded: "contracts.status.superseded",
};

// A terminal status has no valid transition out of it other than a
// same-status no-op (refuseInvalidTransition, contract_lifecycle.go), so
// offering "change status" or "cancel" from one would be a control that can
// only refuse — the reasoning #3573/#3700 already apply to the plan's write
// controls.
export function isTerminalContractStatus(status: Contract["status"]): boolean {
  return (
    status === "expired" || status === "cancelled" || status === "superseded"
  );
}

type RenewDraft = {
  title: string;
  contractNumber: string;
  valueMinor: number;
  currency: string;
  valueBasis: ValueBasis;
  startsOn: string;
  endsOn: string;
  renewalOn: string;
  noticePeriodDays: string;
  signedOn: string;
};

function renewDraftOf(predecessor: Contract): RenewDraft {
  return {
    // Title and basis are the two fields the successor is likeliest to keep,
    // and both are required by the wire request — prefilled so renewing an
    // unchanged agreement does not mean retyping what it was already called.
    // Everything else the predecessor does NOT hand down: RenewContractRequest
    // inherits only the counterparty (the server derives that), because a
    // renewal is usually a fresh negotiation and an inherited amount or term
    // would be a number nobody actually agreed to this time.
    title: predecessor.title,
    contractNumber: "",
    valueMinor: 0,
    currency: "",
    valueBasis: (predecessor.value_basis as ValueBasis) ?? "total",
    startsOn: "",
    endsOn: "",
    renewalOn: "",
    noticePeriodDays: "",
    signedOn: "",
  };
}

function renewalBody(
  draft: RenewDraft,
): components["schemas"]["RenewContractRequest"] {
  const body: components["schemas"]["RenewContractRequest"] = {
    title: draft.title.trim(),
    value_basis: draft.valueBasis,
    // Stated rather than defaulted, for the same reason contractBody
    // (contractform.tsx) states it on a create: whether the successor renews
    // itself is a fact about the paper, and a field this form quietly omitted
    // would be a guess the record could not distinguish from an answer.
    auto_renew: false,
  };
  if (draft.contractNumber.trim() !== "") {
    body.contract_number = draft.contractNumber.trim();
  }
  if (draft.valueMinor > 0) {
    body.value_minor = draft.valueMinor;
    if (draft.currency !== "") {
      body.currency = draft.currency;
    }
  }
  if (draft.startsOn !== "") {
    body.starts_on = draft.startsOn;
  }
  if (draft.endsOn !== "") {
    body.ends_on = draft.endsOn;
  }
  if (draft.renewalOn !== "") {
    body.renewal_on = draft.renewalOn;
  }
  if (draft.noticePeriodDays !== "") {
    body.notice_period_days = Number(draft.noticePeriodDays);
  }
  if (draft.signedOn !== "") {
    body.signed_on = draft.signedOn;
  }
  return body;
}

// The successor's terms, created in the same transaction that supersedes the
// predecessor — the id and version this call takes are the predecessor's, and
// they never come from a closure: a click that lands after the modal has
// re-rendered for a different row would otherwise renew whichever contract the
// previous render held.
async function renewContract(
  predecessor: Contract,
  draft: RenewDraft,
): Promise<string> {
  const { data, error } = await api.POST("/contracts/{id}/renewal", {
    params: {
      path: { id: predecessor.id },
      ...ifMatch(requireVersion(predecessor.version)),
    },
    body: renewalBody(draft),
  });
  if (error) {
    throwProblem(error);
  }
  return data?.id ?? "";
}

export function ContractRenewModal({
  orgId,
  contract,
  open,
  onClose,
}: Readonly<{
  orgId: string;
  contract: Contract;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<RenewDraft>(renewDraftOf(contract));
  const baseCurrency = useInstallationSettings().data?.base_currency;
  const contractCurrency = draft.currency || baseCurrency || "";

  // Re-seed on open, and when a different row's renewal is what just opened:
  // otherwise the form keeps the previous agreement's title and basis.
  useEffect(() => {
    if (open) {
      setDraft(renewDraftOf(contract));
    }
  }, [open, contract]);

  const renew = useMutation({
    mutationFn: async (submitted: {
      predecessor: Contract;
      draft: RenewDraft;
    }) => renewContract(submitted.predecessor, submitted.draft),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orgContracts", orgId] });
      queryClient.invalidateQueries({ queryKey: ["organization360", orgId] });
      onClose();
    },
  });

  const invalid = draftProblem(draft);

  return (
    <Modal open={open} onClose={onClose} labelledBy={titleId}>
      <h2 id={titleId}>{t("contracts.renew.title")}</h2>
      <p className="t-caption">{t("contracts.renew.hint")}</p>

      <Field label={t("contracts.form.name")} required>
        {(props) => (
          <TextInput
            {...props}
            value={draft.title}
            onChange={(e) => setDraft({ ...draft, title: e.target.value })}
          />
        )}
      </Field>

      <Field label={t("contracts.form.number")}>
        {(props) => (
          <TextInput
            {...props}
            value={draft.contractNumber}
            onChange={(e) =>
              setDraft({ ...draft, contractNumber: e.target.value })
            }
          />
        )}
      </Field>

      <Field label={t("contracts.form.value")}>
        {(props) => (
          <MoneyInput
            {...props}
            min={0}
            currency={contractCurrency}
            valueMinor={draft.valueMinor}
            blankWhenZero
            onChangeMinor={(valueMinor) => setDraft({ ...draft, valueMinor })}
          />
        )}
      </Field>

      <Field label={t("contracts.form.basis")} required>
        {(props) => (
          <Select
            {...props}
            value={draft.valueBasis}
            onChange={(value) =>
              setDraft({ ...draft, valueBasis: value as ValueBasis })
            }
            options={[
              { value: "total", label: t("contracts.basis.total") },
              { value: "annualized_12m", label: t("contracts.basis.annual") },
            ]}
          />
        )}
      </Field>

      <Field label={t("contracts.form.startsOn")}>
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={draft.startsOn}
            onChange={(e) => setDraft({ ...draft, startsOn: e.target.value })}
          />
        )}
      </Field>

      <Field
        label={t("contracts.form.endsOn")}
        hint={t("contracts.form.endsOnHint")}
      >
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={draft.endsOn}
            onChange={(e) => setDraft({ ...draft, endsOn: e.target.value })}
          />
        )}
      </Field>

      <Field label={t("contracts.form.renewalOn")}>
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={draft.renewalOn}
            onChange={(e) => setDraft({ ...draft, renewalOn: e.target.value })}
          />
        )}
      </Field>

      <Field
        label={t("contracts.form.noticeDays")}
        hint={t("contracts.form.noticeDaysHint")}
      >
        {(props) => (
          <TextInput
            {...props}
            type="number"
            min={0}
            value={draft.noticePeriodDays}
            onChange={(e) =>
              setDraft({ ...draft, noticePeriodDays: e.target.value })
            }
          />
        )}
      </Field>

      <Field
        label={t("contracts.form.signedOn")}
        hint={t("contracts.form.signedOnHint")}
      >
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={draft.signedOn}
            onChange={(e) => setDraft({ ...draft, signedOn: e.target.value })}
          />
        )}
      </Field>

      {renew.error && (
        <p className="t-caption" role="alert">
          {problemMessageOf(renew.error, t)}
        </p>
      )}

      <div className="modal-actions">
        <Button onClick={onClose}>{t("create.cancel")}</Button>
        <Button
          variant="primary"
          reason={invalid ? t(invalid) : undefined}
          disabled={renew.isPending || invalid !== null}
          onClick={() =>
            renew.mutate({
              predecessor: contract,
              draft: pricedIn(draft, baseCurrency),
            })
          }
        >
          {t("contracts.renew.submit")}
        </Button>
      </div>
    </Modal>
  );
}

// A status is a fact a human asserted, never inferred from a date — the same
// invariant contract_lifecycle.go's own comment states. This is that assertion's
// only door: one Select, one submit, and the version the row was read at.
async function changeContractStatus(
  contract: Contract,
  status: ContractStatus,
): Promise<Contract> {
  const { data, error } = await api.POST("/contracts/{id}/status", {
    params: {
      path: { id: contract.id },
      ...ifMatch(requireVersion(contract.version)),
    },
    body: { status },
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

export function ContractStatusModal({
  contract,
  open,
  onClose,
}: Readonly<{
  contract: Contract;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<ContractStatus>(
    contract.status ?? "draft",
  );

  useEffect(() => {
    if (open) {
      setStatus(contract.status ?? "draft");
    }
  }, [open, contract]);

  const assert = useMutation({
    mutationFn: async (submitted: {
      contract: Contract;
      status: ContractStatus;
    }) => changeContractStatus(submitted.contract, submitted.status),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["orgContracts", contract.organization_id],
      });
      queryClient.invalidateQueries({
        queryKey: ["organization360", contract.organization_id],
      });
      onClose();
    },
  });

  return (
    <Modal open={open} onClose={onClose} labelledBy={titleId}>
      <h2 id={titleId}>{t("contracts.statusChange.title")}</h2>

      <Field label={t("contracts.statusChange.label")}>
        {(props) => (
          <Select
            {...props}
            value={status}
            onChange={(value) => setStatus(value as ContractStatus)}
            options={ASSERTABLE_STATUSES.map((value) => ({
              value,
              label: t(STATUS_LABEL_KEY[value]),
            }))}
          />
        )}
      </Field>

      {assert.error && (
        <p className="t-caption" role="alert">
          {problemMessageOf(assert.error, t)}
        </p>
      )}

      <div className="modal-actions">
        <Button onClick={onClose}>{t("create.cancel")}</Button>
        <Button
          variant="primary"
          disabled={assert.isPending}
          onClick={() => assert.mutate({ contract, status })}
        >
          {t("contracts.statusChange.submit")}
        </Button>
      </div>
    </Modal>
  );
}

// Cancellation is two facts and NO state change — the same invariant
// Store.Cancel's own comment states. The customer stays under contract until
// the effective date; only the row's "ends" reading moves, not its status.
async function cancelContract(
  contract: Contract,
  noticeOn: string,
  effectiveOn: string,
): Promise<Contract> {
  const { data, error } = await api.POST("/contracts/{id}/cancellation", {
    params: {
      path: { id: contract.id },
      ...ifMatch(requireVersion(contract.version)),
    },
    body: {
      cancellation_notice_on: noticeOn,
      cancellation_effective_on: effectiveOn,
    },
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

export function ContractCancelModal({
  contract,
  open,
  onClose,
}: Readonly<{
  contract: Contract;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const queryClient = useQueryClient();
  const [noticeOn, setNoticeOn] = useState("");
  const [effectiveOn, setEffectiveOn] = useState("");

  useEffect(() => {
    if (open) {
      setNoticeOn(contract.cancellation_notice_on ?? "");
      setEffectiveOn(contract.cancellation_effective_on ?? "");
    }
  }, [open, contract]);

  const cancel = useMutation({
    mutationFn: async (submitted: {
      contract: Contract;
      noticeOn: string;
      effectiveOn: string;
    }) =>
      cancelContract(
        submitted.contract,
        submitted.noticeOn,
        submitted.effectiveOn,
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["orgContracts", contract.organization_id],
      });
      queryClient.invalidateQueries({
        queryKey: ["organization360", contract.organization_id],
      });
      onClose();
    },
  });

  const invalid =
    noticeOn === "" || effectiveOn === ""
      ? "contracts.cancel.errIncomplete"
      : effectiveOn < noticeOn
        ? "contracts.cancel.errOrder"
        : null;

  return (
    <Modal open={open} onClose={onClose} labelledBy={titleId}>
      <h2 id={titleId}>{t("contracts.cancel.title")}</h2>
      <p className="t-caption">{t("contracts.cancel.hint")}</p>

      <Field label={t("contracts.cancel.noticeOn")} required>
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={noticeOn}
            onChange={(e) => setNoticeOn(e.target.value)}
          />
        )}
      </Field>

      <Field
        label={t("contracts.cancel.effectiveOn")}
        hint={t("contracts.cancel.effectiveOnHint")}
        required
      >
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={effectiveOn}
            onChange={(e) => setEffectiveOn(e.target.value)}
          />
        )}
      </Field>

      {cancel.error && (
        <p className="t-caption" role="alert">
          {problemMessageOf(cancel.error, t)}
        </p>
      )}

      <div className="modal-actions">
        <Button onClick={onClose}>{t("create.cancel")}</Button>
        <Button
          variant="danger"
          reason={invalid ? t(invalid) : undefined}
          disabled={cancel.isPending || invalid !== null}
          onClick={() => cancel.mutate({ contract, noticeOn, effectiveOn })}
        >
          {t("contracts.cancel.submit")}
        </Button>
      </div>
    </Modal>
  );
}
