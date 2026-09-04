// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useInstallationSettings } from "../app/uploadlimit";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import {
  type ContractDraft,
  ContractTermsFields,
  contractTermsBody,
  draftProblem,
  pricedIn,
} from "./contractform";

// margince#3286: the three transitions a signed agreement actually goes
// through after it is first recorded — renew, assert a status, record a
// cancellation. Store.Renew / ChangeStatus / Cancel and their endpoints
// (POST /contracts/{id}/renewal /status /cancellation) have always been
// correct; nothing in the app could reach them.

type Contract = components["schemas"]["Contract"];
type ContractStatus = NonNullable<Contract["status"]>;
type ValueBasis = ContractDraft["valueBasis"];

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
// offering "change status" from one would be a control that can only refuse —
// the reasoning #3573/#3700 already apply to the plan's write controls.
// Cancel is NOT gated on this (companycontracts.tsx): Store.Cancel is a plain
// column patch with no status check at all.
export function isTerminalContractStatus(status: Contract["status"]): boolean {
  return (
    status === "expired" || status === "cancelled" || status === "superseded"
  );
}

function renewDraftOf(predecessor: Contract): ContractDraft {
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
  draft: ContractDraft,
): components["schemas"]["RenewContractRequest"] {
  return {
    title: draft.title.trim(),
    value_basis: draft.valueBasis,
    // Stated rather than defaulted, for the same reason contractBody
    // (contractform.tsx) states it on a create: whether the successor renews
    // itself is a fact about the paper, and a field this form quietly omitted
    // would be a guess the record could not distinguish from an answer.
    auto_renew: false,
    ...contractTermsBody(draft),
  };
}

// The successor's terms, created in the same transaction that supersedes the
// predecessor — the id and version this call takes are the predecessor's, and
// they never come from a closure: a click that lands after the modal has
// re-rendered for a different row would otherwise renew whichever contract the
// previous render held.
async function renewContract(
  predecessor: Contract,
  draft: ContractDraft,
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
  const [draft, setDraft] = useState<ContractDraft>(renewDraftOf(contract));
  const baseCurrency = useInstallationSettings().data?.base_currency;
  const contractCurrency = draft.currency || baseCurrency || "";

  // Re-seed on open, and when a different row's renewal is what just opened:
  // otherwise the form keeps the previous agreement's title and basis.
  //
  // Keyed on the ID, never the CONTRACT OBJECT: react-query hands back a new
  // object on every refetch of the same row even when nothing changed, and a
  // background refetch while this modal is open would otherwise re-seed
  // mid-edit and discard whatever the reader had already typed.
  // biome-ignore lint/correctness/useExhaustiveDependencies: contract.id decides whether to reseed; the object itself would reseed on every refetch of the same row, discarding an in-progress edit.
  useEffect(() => {
    if (open) {
      setDraft(renewDraftOf(contract));
    }
  }, [open, contract.id]);

  const renew = useMutation({
    mutationFn: async (submitted: {
      predecessor: Contract;
      draft: ContractDraft;
    }) => renewContract(submitted.predecessor, submitted.draft),
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

  const invalid = draftProblem(draft);

  return (
    <Modal open={open} onClose={onClose} labelledBy={titleId}>
      <h2 id={titleId}>{t("contracts.renew.title")}</h2>
      <p className="t-caption">{t("contracts.renew.hint")}</p>

      <ContractTermsFields
        draft={draft}
        setDraft={setDraft}
        currency={contractCurrency}
      />

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

  // Keyed on the ID, never the CONTRACT OBJECT — see ContractRenewModal's
  // identical comment: a background refetch of the SAME row must not discard
  // a status the reader already picked.
  // biome-ignore lint/correctness/useExhaustiveDependencies: contract.id decides whether to reseed; the object itself would reseed on every refetch of the same row, discarding an in-progress edit.
  useEffect(() => {
    if (open) {
      setStatus(contract.status ?? "draft");
    }
  }, [open, contract.id]);

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
        {/* recordAssignment (patch.go) records a SET regardless of whether
            the new value equals the old one, so a same-status submit would
            still bump the row's version and write an audit row + a from==to
            contract.status_changed event — a write nobody asked for. */}
        <Button
          variant="primary"
          reason={
            status === contract.status
              ? t("contracts.statusChange.errSame")
              : undefined
          }
          disabled={assert.isPending || status === contract.status}
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

  // Keyed on the ID, never the CONTRACT OBJECT — see ContractRenewModal's
  // identical comment: a background refetch of the SAME row must not discard
  // dates the reader already typed.
  // biome-ignore lint/correctness/useExhaustiveDependencies: contract.id decides whether to reseed; the object itself would reseed on every refetch of the same row, discarding an in-progress edit.
  useEffect(() => {
    if (open) {
      setNoticeOn(contract.cancellation_notice_on ?? "");
      setEffectiveOn(contract.cancellation_effective_on ?? "");
    }
  }, [open, contract.id]);

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

  // Mirrors the two CHECK constraints contractCheckError names
  // (contract_lifecycle.go): a cancellation cannot take effect before notice
  // was given, and not after the term already ends. Held here too so the
  // control does not enable a submit the server is certain to refuse.
  const invalid =
    noticeOn === "" || effectiveOn === ""
      ? "contracts.cancel.errIncomplete"
      : effectiveOn < noticeOn
        ? "contracts.cancel.errOrder"
        : contract.ends_on && effectiveOn > contract.ends_on
          ? "contracts.cancel.errTermEnd"
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
