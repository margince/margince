import { useMutation } from "@tanstack/react-query";
import { useId, useState } from "react";
import type { components } from "../api/schema";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { stillHeld } from "./employmentcurrency";
import { datePatch, patchEmployment, validDateEntry } from "./employmentpatch";
import "./common.css";

type Employment = components["schemas"]["Contact360Employment"];
type Patch = components["schemas"]["UpdateRelationshipRequest"];

export function EmploymentEdit({
  employment,
  open = true,
  contactId,
  onClose,
  onSaved,
}: Readonly<{
  employment: Employment;
  /**
   * Whether the dialog is showing. Closed, it stays MOUNTED so it can animate
   * out — which is why the caller keeps handing it the employment it was
   * opened on. Defaults to open: a caller drawing this on its own, a story
   * included, is drawing an open dialog.
   */
  open?: boolean;
  contactId: string;
  onClose: () => void;
  onSaved: () => Promise<void>;
}>) {
  const t = useT();
  const id = useId();
  const [primary, setPrimary] = useState(employment.is_current_primary);
  const [role, setRole] = useState(employment.role ?? "");
  const [status, setStatus] = useState(
    stillHeld(employment)
      ? "current"
      : (employment.employment_status ?? "former"),
  );
  const [start, setStart] = useState(
    employment.started_at?.slice(
      0,
      employment.started_precision === "month" ? 7 : 10,
    ) ?? "",
  );
  const [end, setEnd] = useState(
    employment.ended_at?.slice(
      0,
      employment.ended_precision === "month" ? 7 : 10,
    ) ?? "",
  );
  const saving = useMutation({
    mutationFn: ({
      original,
      subject,
      body,
    }: {
      original: Employment;
      subject: string;
      body: Patch;
    }) => patchEmployment(original, subject, body, t),
    onSuccess: async () => {
      await onSaved();
      onClose();
    },
  });
  const valid = validDateEntry(start) && validDateEntry(end);
  return (
    <Modal open={open} onClose={onClose} labelledBy={id}>
      <Heading size="large" id={id} className="t-h2 dialog-heading">
        {t("employment.edit")}
      </Heading>
      <div className="form-stack">
        <Field label={t("rel.role")}>
          {(control) => (
            <TextInput
              {...control}
              value={role}
              onChange={(e) => setRole(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("employment.statusLabel")}>
          {(control) => (
            <Select
              {...control}
              value={status}
              onChange={(value) => {
                if (
                  value === "current" ||
                  value === "former" ||
                  value === "unknown"
                )
                  setStatus(value);
              }}
              options={["current", "former", "unknown"].map((value) => ({
                value,
                label: t(
                  value === "current"
                    ? "employment.status.current"
                    : value === "former"
                      ? "employment.status.former"
                      : "employment.status.unknown",
                ),
              }))}
            />
          )}
        </Field>
        <Field label={t("employment.start")} hint={t("employment.dateHint")}>
          {(control) => (
            <TextInput
              {...control}
              value={start}
              onChange={(e) => setStart(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("employment.end")} hint={t("employment.dateHint")}>
          {(control) => (
            <TextInput
              {...control}
              value={end}
              onChange={(e) => setEnd(e.target.value)}
            />
          )}
        </Field>
        <Checkbox
          label={t("contact.rail.isCurrentEmployer")}
          checked={status === "current" && primary}
          disabled={status !== "current" || saving.isPending}
          onChange={(e) => setPrimary(e.target.checked)}
        />
        <ErrorLine error={saving.error} />
        <Button
          disabled={!valid || saving.isPending}
          onClick={() => {
            const started = datePatch(start),
              ended = datePatch(end);
            const body: Patch = {
              role: role === (employment.role ?? "") ? undefined : role,
              employment_status: status,
              is_current_primary: status === "current" && primary,
              started_at: started.date,
              ended_at: ended.date,
              started_precision: started.precision,
              ended_precision: ended.precision,
              clear_started_at:
                !start && employment.started_at ? true : undefined,
              clear_ended_at: !end && employment.ended_at ? true : undefined,
            };
            saving.mutate({ original: employment, subject: contactId, body });
          }}
        >
          {t("record.save")}
        </Button>
      </div>
    </Modal>
  );
}
