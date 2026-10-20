import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, ShieldCheck } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import {
  Button,
  Field,
  Modal,
  SectionHeader,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { WriteRefused } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { coldFieldLabel } from "../settings/coldfieldlabels";
import {
  companySaveMessage,
  storeCompany,
} from "../sign-in/installationcompany";
import type { CompanyInput } from "./company-context";
import { PROFILE_GROUPS } from "./companycontextfacts";

const EMPTY_COMPANY_INPUT: CompanyInput = {
  display_name: "",
  website: "",
  offer_summary: "",
  icp: "",
};

const MULTILINE_FIELDS = new Set<keyof CompanyInput>([
  "offer_summary",
  "icp",
  "value_proposition",
  "customer_pains",
  "desired_outcomes",
  "buying_center",
  "buying_intents",
  "common_objections",
  "sales_motion",
  "history",
]);

// ManualCompanySetup is the rollback-safe first-run floor below the
// `onboarding` rollout stage. It creates the same canonical profile with the
// same semantic minimum, without exposing the five-step experience.
export function ManualCompanySetup() {
  const t = useT();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<CompanyInput>(EMPTY_COMPANY_INPUT);
  const save = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.PUT("/company", {
          body: trimCompanyInput(form),
        }),
      );
    },
    onSuccess: (profile) => {
      storeCompany(queryClient, profile);
      navigate({ screen: "home" });
    },
  });
  return (
    // The heading is the panel's own title rather than a bare h2: preflight
    // leaves an unclassed heading at body size.
    <div className="wrap narrow">
      <Panel
        tone="accent"
        title={
          <>
            <ShieldCheck aria-hidden size={16} />
            {t("settings.companyManualTitle")}
          </>
        }
        actions={
          <Button
            variant="primary"
            disabled={!requiredComplete(form) || save.isPending}
            onClick={() => save.mutate()}
          >
            {t("settings.companyCreateWorkspace")} <ArrowRight aria-hidden />
          </Button>
        }
      >
        <PanelBody className="form-stack">
          <Eyebrow>{t("settings.companyManualKicker")}</Eyebrow>
          <p>{t("settings.companyManualSub")}</p>
          {(["display_name", "offer_summary", "icp"] as const).map((field) => (
            <Field key={field} label={coldFieldLabel(field, t)}>
              {(control) =>
                field === "display_name" ? (
                  <TextInput
                    {...control}
                    value={String(form[field] ?? "")}
                    onChange={(event) =>
                      setForm({ ...form, [field]: event.target.value })
                    }
                  />
                ) : (
                  <Textarea
                    {...control}
                    rows={4}
                    value={String(form[field] ?? "")}
                    onChange={(event) =>
                      setForm({ ...form, [field]: event.target.value })
                    }
                  />
                )
              }
            </Field>
          ))}
          {save.isError && (
            <WriteRefused
              titleKey="settings.companySaveFailed"
              message={companySaveMessage(save.error, t)}
            />
          )}
        </PanelBody>
      </Panel>
    </div>
  );
}

/**
 * The one form every row's Edit verb opens. One drawer, because one PUT writes
 * the profile: a dialog per group would commit the others' unsaved drafts.
 */
export function CompanyProfileDialog({
  form,
  focus,
  pending,
  error,
  onChange,
  onClose,
  onSubmit,
}: Readonly<{
  form: CompanyInput;
  /** Which row's Edit was pressed, and so where focus lands. */
  focus: keyof CompanyInput;
  pending: boolean;
  error: string | null;
  onChange: (next: CompanyInput) => void;
  onClose: () => void;
  onSubmit: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  // Not `autoFocus`: the a11y lint refuses it. A callback, because the field
  // it lands on is an input for some rows and a textarea for others.
  const asked = useRef<HTMLElement | null>(null);
  const capture = (node: HTMLElement | null) => {
    asked.current = node;
  };
  useEffect(() => {
    asked.current?.focus();
  }, []);
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2">
          {t("settings.companyContextTitle")}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          {/* The site the read starts from, above the statements a read would
              propose changes to: it is the precondition for everything below. */}
          <CompanyFieldInput
            field="website"
            form={form}
            asked={focus === "website" ? capture : undefined}
            onChange={onChange}
          />
          {PROFILE_GROUPS.map((group) => (
            <div className="form-stack" key={group.title}>
              <SectionHeader title={t(group.title)} level={3} />
              {group.fields.map((field) => (
                <CompanyFieldInput
                  key={field}
                  field={field}
                  form={form}
                  asked={focus === field ? capture : undefined}
                  onChange={onChange}
                />
              ))}
            </div>
          ))}
          {error !== null && (
            <WriteRefused
              titleKey="settings.companySaveFailed"
              message={error}
            />
          )}
        </form>
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button variant="ghost" type="button" onClick={onClose}>
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={!pending && !requiredComplete(form)}
          pending={pending}
          busyLabel={t("common.saving")}
        >
          {t("settings.companySave")}
        </Button>
      </DrawerFoot>
    </Modal>
  );
}

/** One field of that form; `Field` draws the real `<label for>`. */
function CompanyFieldInput({
  field,
  form,
  asked,
  onChange,
}: Readonly<{
  field: keyof CompanyInput;
  form: CompanyInput;
  /** Set on the one field whose Edit opened this dialog, so focus lands there. */
  asked?: (node: HTMLElement | null) => void;
  onChange: (next: CompanyInput) => void;
}>) {
  const t = useT();
  const value = String(form[field] ?? "");
  const label =
    field === "website"
      ? t("settings.companyWebsite")
      : coldFieldLabel(field, t);
  return (
    <Field label={label}>
      {(control) =>
        MULTILINE_FIELDS.has(field) ? (
          <Textarea
            {...control}
            ref={asked}
            rows={3}
            value={value}
            onChange={(event) =>
              onChange({ ...form, [field]: event.target.value })
            }
          />
        ) : (
          <TextInput
            {...control}
            ref={asked}
            value={value}
            onChange={(event) =>
              onChange({ ...form, [field]: event.target.value })
            }
          />
        )
      }
    </Field>
  );
}

function requiredComplete(form: CompanyInput): boolean {
  return [form.display_name, form.offer_summary, form.icp].every(
    (value) => String(value ?? "").trim() !== "",
  );
}

export function trimCompanyInput(form: CompanyInput): CompanyInput {
  return Object.fromEntries(
    Object.entries(form).map(([key, value]) => [
      key,
      typeof value === "string" ? value.trim() : value,
    ]),
  ) as CompanyInput;
}
