import { useMutation } from "@tanstack/react-query";
import { useId, useState } from "react";
import type { components } from "../../api/schema";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  TextInput,
} from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useFieldFaults } from "../../design-system/savepage";
import { useLocale, useT } from "../../i18n";
import { stillHeld, useEmploymentZone } from "../../lib/employmentcurrency";
import { blankRequired } from "../../lib/problem";
import { datePatch, patchEmployment } from "./employmentpatch";
import {
  type EmploymentStatusChoice,
  EmploymentStatusFields,
  employmentDateFaults,
} from "./employmentstatusfields";
import "../../design-system/querystates.css";

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
  const { locale } = useLocale();
  const id = useId();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const employmentZone = useEmploymentZone();
  const [primary, setPrimary] = useState(employment.is_current_primary);
  const [role, setRole] = useState(employment.role ?? "");
  const [status, setStatus] = useState<EmploymentStatusChoice>(
    stillHeld(employment, employmentZone)
      ? "current"
      : employment.employment_status === "unknown"
        ? ""
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
  return (
    <Modal open={open} onClose={onClose} labelledBy={id} intent="form">
      <Heading size="large" id={id} className="t-h2 modal-title">
        {t("employment.edit")}
      </Heading>
      <div className="form-stack" ref={faultsRoot}>
        <Field label={t("rel.role")}>
          {(control) => (
            <TextInput
              {...control}
              value={role}
              onChange={(e) => setRole(e.target.value)}
            />
          )}
        </Field>
        <Field
          label={t("employment.start")}
          hint={t("employment.dateHint")}
          error={faults.of("start")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={start}
              onChange={(e) => {
                setStart(e.target.value);
                faults.edited("start");
              }}
            />
          )}
        </Field>
        <EmploymentStatusFields
          status={status}
          onStatusChange={(next) => {
            setStatus(next);
            faults.edited("status");
          }}
          end={end}
          onEndChange={(next) => {
            setEnd(next);
            faults.edited("end");
          }}
          statusError={faults.of("status")}
          endError={faults.of("end")}
        />
        <Checkbox
          label={t("contact.rail.isPrimaryEmployer")}
          checked={status === "current" && primary}
          disabled={status !== "current" || saving.isPending}
          onChange={(e) => setPrimary(e.target.checked)}
        />
        <ErrorLine error={saving.error} />
      </div>
      <div className="actions">
        <Button onClick={onClose} disabled={saving.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          disabled={saving.isPending}
          onClick={() => {
            const found = new Map([
              ...blankRequired(locale, {
                status: [t("employment.statusLabel"), status],
              }),
              ...employmentDateFaults(locale, t, { start, end }),
            ]);
            if (faults.show(found)) return;
            const started = datePatch(start),
              ended = datePatch(end);
            const body: Patch = {
              role: role === (employment.role ?? "") ? undefined : role,
              employment_status: status || undefined,
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
