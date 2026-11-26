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
import { type FieldFaults, useFieldFaults } from "../../design-system/savepage";
import { type Locale, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { coldFieldLabel } from "../../lib/coldfieldlabels";
import {
  COMPANY_NAME_MAX,
  companyRefusal,
  storeCompany,
} from "../../lib/installationcompany";
import { fromHere } from "../../lib/navigatefrom";
import { fieldCheckSentence, unwrap } from "../../lib/problem";
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
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<CompanyInput>(EMPTY_COMPANY_INPUT);
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const save = useMutation({
    mutationFn: async (body: CompanyInput) => {
      return unwrap(await api.PUT("/company", { body }));
    },
    onError: (error) => faults.show(companyRefusal(error, t).byField),
    onMutate: fromHere,
    // Replacing: the first-run form is spent once the profile exists.
    onSuccess: (profile, _vars, from) => {
      storeCompany(queryClient, profile);
      from.navigateReplacing({ screen: "home" });
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
            pending={save.isPending}
            onClick={() => {
              if (faults.show(missingCompanyFields(form, t, locale))) {
                save.reset();
                return;
              }
              save.mutate(trimCompanyInput(form));
            }}
          >
            {t("settings.companyCreateWorkspace")} <ArrowRight aria-hidden />
          </Button>
        }
      >
        <PanelBody className="form-stack">
          <Eyebrow>{t("settings.companyManualKicker")}</Eyebrow>
          <p>{t("settings.companyManualSub")}</p>
          <div className="form-stack" ref={faultsRoot}>
            {REQUIRED_FIELDS.map((field) => (
              <CompanyFieldInput
                key={field}
                field={field}
                form={form}
                faults={faults}
                onChange={setForm}
              />
            ))}
          </div>
          <WriteRefused
            titleKey="settings.companySaveFailed"
            message={companyRefusal(save.error, t).rest}
          />
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
  refusedAt,
  dirty,
  onChange,
  onClose,
  onSubmit,
}: Readonly<{
  form: CompanyInput;
  /** Which row's Edit was pressed, and so where focus lands. */
  focus: keyof CompanyInput;
  pending: boolean;
  /** The refused save, and when it was sent, so each refusal is shown once. */
  error: unknown;
  refusedAt: number | undefined;
  dirty: boolean;
  onChange: (next: CompanyInput) => void;
  /** Discards the draft. Cancel calls it at once; the X, Escape and the backdrop ask first. */
  onClose: () => void;
  onSubmit: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const titleId = useId();
  const formId = useId();
  const [faults, faultsRoot] = useFieldFaults();
  const refused = companyRefusal(error, t);
  const [refusalShown, setRefusalShown] = useState<number>();
  if (refusedAt !== refusalShown) {
    setRefusalShown(refusedAt);
    faults.show(refusedAt === undefined ? new Map() : refused.byField);
  }
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
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer"
      unsaved={dirty}
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2">
          {t("settings.companyContextTitle")}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <form
          id={formId}
          ref={faultsRoot}
          className="form-stack"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            if (pending) return;
            if (!faults.show(missingCompanyFields(form, t, locale))) {
              onSubmit();
            }
          }}
        >
          {/* The site the read starts from, above the statements a read would
              propose changes to: it is the precondition for everything below. */}
          <CompanyFieldInput
            field="website"
            form={form}
            faults={faults}
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
                  faults={faults}
                  asked={focus === field ? capture : undefined}
                  onChange={onChange}
                />
              ))}
            </div>
          ))}
          <WriteRefused
            titleKey="settings.companySaveFailed"
            message={refused.rest}
          />
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
  faults,
  asked,
  onChange,
}: Readonly<{
  field: keyof CompanyInput;
  form: CompanyInput;
  faults: Pick<FieldFaults, "of" | "edited">;
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
  const change = (next: string) => {
    onChange({ ...form, [field]: next });
    faults.edited(field);
  };
  return (
    <Field label={label} required={isRequired(field)} error={faults.of(field)}>
      {(control) =>
        MULTILINE_FIELDS.has(field) ? (
          <Textarea
            {...control}
            ref={asked}
            rows={3}
            value={value}
            onChange={(event) => change(event.target.value)}
          />
        ) : (
          <TextInput
            {...control}
            ref={asked}
            maxLength={field === "display_name" ? COMPANY_NAME_MAX : undefined}
            value={value}
            onChange={(event) => change(event.target.value)}
          />
        )
      }
    </Field>
  );
}

// The semantic minimum a company profile needs before it is saved.
const REQUIRED_FIELDS = ["display_name", "offer_summary", "icp"] as const;

const REQUIRED_SET: ReadonlySet<keyof CompanyInput> = new Set<
  keyof CompanyInput
>(REQUIRED_FIELDS);

function isRequired(field: keyof CompanyInput): boolean {
  return REQUIRED_SET.has(field);
}

function missingCompanyFields(
  form: CompanyInput,
  t: (key: MessageKey) => string,
  locale: Locale,
): ReadonlyMap<string, string> {
  return new Map(
    REQUIRED_FIELDS.filter(
      (field) => String(form[field] ?? "").trim() === "",
    ).map((field) => [
      field,
      fieldCheckSentence("required", locale, coldFieldLabel(field, t)),
    ]),
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
