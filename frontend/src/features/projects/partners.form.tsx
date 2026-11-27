import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { SUBMIT_COPY, type SubmitIntent } from "../../components/create";
import {
  Button,
  EmptyState,
  Field,
  SectionHeader,
  TextInput,
} from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { useFieldFaults } from "../../design-system/savepage";
import { MultiSelect, Select } from "../../design-system/select";
import { useLocale, useT } from "../../i18n";
import {
  useCanWrite,
  useGrantRefused,
  useWriteRefusal,
} from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
import { isOption } from "../../lib/options";
import {
  assignableStages,
  KIND_LABELS,
  PARTNER_KINDS,
  type PartnerKind,
  partnerKindsLabel,
  partnerStageLabel,
  usePartnerStages,
} from "../../lib/partnerstages.queries";
import { blankRequired, throwProblem, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";
import { PartnerCommissions } from "./partnercommissions";
import { PartnerDeals } from "./partnerdeals";
import { ActivationNote, MarginTierRow } from "./partners.detailrows";
import { EndPartnerProgramAction } from "./partners.end";
import { PartnerNextStep } from "./partners.nextstep";
import {
  CERT_LABELS,
  CERT_STATUSES,
  type CertStatus,
  MARGIN_TIER_LABELS,
  MARGIN_TIERS,
  type MarginTier,
  type Partner,
  type UpsertPartnerRequest,
} from "./partners.vocab";

async function fetchPartner(companyId: string): Promise<Partner | null> {
  const { data, error, response } = await api.GET("/companies/{id}/partner", {
    params: { path: { id: companyId } },
  });
  if (response.status === 404) {
    return null;
  }
  if (error) {
    throwProblem(error);
  }
  return data ?? null;
}

type PartnerFormValues = {
  partner_kinds: PartnerKind[];
  cert_status: CertStatus;
  margin_tier: "" | MarginTier;
  // Empty on a new programme: the server starts it on the first candidate stage.
  relationship_stage: string;
  served_segments: string;
};

function defaultFormValues(partner?: Partner): PartnerFormValues {
  return {
    partner_kinds: [...(partner?.partner_kinds ?? [])],
    cert_status: partner?.cert_status ?? "applied",
    margin_tier: partner?.margin_tier ?? "",
    relationship_stage: partner?.relationship_stage ?? "",
    served_segments: (partner?.served_segments ?? []).join(", "),
  };
}

// A seat without the partner grant leaves the tier out of the body. The server
// takes the grant for any tier the body names, a null included. The old
// free-text next step is never sent, so a save keeps what is stored.
function buildUpsertBody(
  values: PartnerFormValues,
  setsTier: boolean,
): UpsertPartnerRequest {
  const segments = values.served_segments
    .split(",")
    .map((segment) => segment.trim())
    .filter((segment) => segment.length > 0);
  return {
    partner_kinds: values.partner_kinds,
    cert_status: values.cert_status,
    ...(setsTier ? { margin_tier: values.margin_tier || null } : {}),
    relationship_stage: values.relationship_stage || undefined,
    served_segments: segments.length > 0 ? segments : undefined,
  };
}

// The one form both "make this a partner" and "edit partner" render. They
// differ only in the record they prefill from and whether the PUT carries
// If-Match. First creation has no prior version to pin, so it sends none.
function PartnerForm({
  companyId,
  partner,
  onSaved,
  onCancel,
  intent,
  dialogTitle,
}: Readonly<{
  companyId: string;
  partner?: Partner;
  onSaved: () => void;
  onCancel?: () => void;
  intent: SubmitIntent;
  /** Draws the form open in a dialog with this title, instead of in the page. */
  dialogTitle?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults();
  const formId = useId();
  const kindsLabel = t("partner.kinds");
  // This form mounts only while editing (PartnerDetail/PartnerTab remount it
  // each time `editing` flips true), so the lazy initializer is all the
  // seeding it needs. A re-sync effect keyed on `partner` would re-run on a
  // background refetch of ["partner", companyId] and overwrite the user's
  // typing.
  const [values, setValues] = useState<PartnerFormValues>(() =>
    defaultFormValues(partner),
  );
  const setsTier = useCanWrite("partner", "update");
  const tierRefused = useGrantRefused({ object: "partner", action: "update" });
  const stages = assignableStages(
    usePartnerStages().data,
    partner?.relationship_stage,
  );

  const initialStage =
    stages.find((stage) => stage.active && stage.meaning === "candidate")
      ?.key ?? "";
  const stage = values.relationship_stage || initialStage;
  const stageNeedsActivation = !partner?.activated_at;

  const mutation = useMutation({
    // The prior row rides the variables rather than the closure, and its
    // presence decides the precondition. First creation has no version to pin.
    // A replacement pins the one the form was filled from, and refuses rather
    // than upserting over an edit it never saw.
    mutationFn: async (prior: Partner | undefined) => {
      return unwrap(
        await api.PUT("/companies/{id}/partner", {
          params: {
            path: { id: companyId },
            ...(prior === undefined
              ? {}
              : ifMatch(requireVersion(prior.version))),
          },
          body: buildUpsertBody(
            { ...values, relationship_stage: stage },
            setsTier,
          ),
        }),
      );
    },
    onSuccess: onSaved,
  });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const blank = blankRequired(locale, {
      partner_kinds: [kindsLabel, values.partner_kinds.join(",")],
    });
    if (faults.show(blank)) return;
    if (!refuseUnreadableField(event.currentTarget)) {
      mutation.mutate(partner);
    }
  };
  const fields = (
    <>
      <Field label={kindsLabel} required error={faults.of("partner_kinds")}>
        {(control) => (
          <MultiSelect
            {...control}
            values={values.partner_kinds}
            onChange={(picked) => {
              faults.edited("partner_kinds");
              setValues({
                ...values,
                partner_kinds: PARTNER_KINDS.filter((kind) =>
                  picked.includes(kind),
                ),
              });
            }}
            options={PARTNER_KINDS.map((kind) => ({
              value: kind,
              label: t(labelFor(KIND_LABELS, kind)),
            }))}
          />
        )}
      </Field>
      <Field label={t("partner.certStatus")}>
        {(control) => (
          <Select
            {...control}
            value={values.cert_status}
            onChange={(value) =>
              isOption(value, CERT_STATUSES) &&
              setValues({ ...values, cert_status: value })
            }
            options={CERT_STATUSES.map((status) => ({
              value: status,
              label: t(labelFor(CERT_LABELS, status)),
            }))}
          />
        )}
      </Field>
      <Field
        label={t("partner.marginTier")}
        hint={t(
          tierRefused ? "partner.marginTierRefused" : "partner.marginTierHint",
        )}
      >
        {(control) => (
          <Select
            {...control}
            disabled={!setsTier}
            value={values.margin_tier}
            onChange={(value) =>
              setValues({
                ...values,
                margin_tier: isOption(value, MARGIN_TIERS) ? value : "",
              })
            }
            // The clearing entry is a real choice, not a placeholder. A tier once
            // set has to be clearable back to "no tier agreed" (the wire's null).
            // It carries a label, because a blank one is an unreadable strip in a
            // drawn list and says nothing to a screen reader.
            options={[
              { value: "", label: t("field.unset") },
              ...MARGIN_TIERS.map((tier) => ({
                value: tier,
                label: t(labelFor(MARGIN_TIER_LABELS, tier)),
              })),
            ]}
          />
        )}
      </Field>
      <Field
        label={t("partner.stage")}
        hint={
          stageNeedsActivation ? t("partner.stageNeedsActivation") : undefined
        }
      >
        {(control) => (
          <Select
            {...control}
            value={stage}
            onChange={(value) =>
              setValues({ ...values, relationship_stage: value })
            }
            options={stages.map((stage) => ({
              value: stage.key,
              label: stage.label,
              disabled: stageNeedsActivation && stage.meaning === "active",
            }))}
          />
        )}
      </Field>
      <Field
        label={t("partner.servedSegments")}
        hint={t("partner.servedSegmentsHint")}
      >
        {(control) => (
          <TextInput
            {...control}
            value={values.served_segments}
            onChange={(event) =>
              setValues({ ...values, served_segments: event.target.value })
            }
          />
        )}
      </Field>
      <ErrorLine error={mutation.error} />
    </>
  );
  const actions = (
    <>
      {onCancel && (
        <Button
          type="button"
          onClick={onCancel}
          disabled={dialogTitle !== undefined && mutation.isPending}
        >
          {t("create.cancel")}
        </Button>
      )}
      <Button
        variant="primary"
        type="submit"
        form={formId}
        pending={mutation.isPending}
        busyLabel={t(SUBMIT_COPY[intent].busy)}
      >
        {t(SUBMIT_COPY[intent].label)}
      </Button>
    </>
  );
  if (dialogTitle !== undefined) {
    return (
      <RecordFormDialog
        open
        title={dialogTitle}
        onClose={() => onCancel?.()}
        fieldCount={5}
        unsaved={
          !mutation.isPending &&
          JSON.stringify(values) !== JSON.stringify(defaultFormValues(partner))
        }
        closeDisabled={mutation.isPending}
        form={
          <form
            id={formId}
            ref={faultsRoot}
            onSubmit={submit}
            className="form-stack"
          >
            {fields}
          </form>
        }
        actions={actions}
      />
    );
  }
  return (
    <form id={formId} ref={faultsRoot} onSubmit={submit} className="form-stack">
      {fields}
      <div className="form-actions">{actions}</div>
    </form>
  );
}

function PartnerDetail({
  companyId,
  partner,
  refusal,
  onSaved,
}: Readonly<{
  companyId: string;
  partner: Partner;
  refusal: string | undefined;
  onSaved: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const [editing, setEditing] = useState(false);

  return (
    <Panel
      title={t("tab.partner")}
      actions={
        <EndPartnerProgramAction
          companyId={companyId}
          version={partner.version}
          refusal={refusal}
          onEnded={onSaved}
        />
      }
      titleAction={
        <Button
          onClick={() => setEditing(true)}
          reason={refusal}
          data-testid="edit-partner"
        >
          {t("record.edit")}
        </Button>
      }
    >
      <PanelBody>
        <dl className="detail-grid">
          {(partner.partner_kinds ?? []).length > 0 && (
            <>
              <dt>{t("partner.kinds")}</dt>
              <dd>{partnerKindsLabel(partner.partner_kinds, t)}</dd>
            </>
          )}
          <dt>{t("partner.certStatus")}</dt>
          <dd>{t(labelFor(CERT_LABELS, partner.cert_status))}</dd>
          <MarginTierRow partner={partner} />
          <dt>{t("partner.stage")}</dt>
          <dd>
            {partnerStageLabel(partner, t)}
            <ActivationNote partner={partner} locale={locale} zone={zone} />
          </dd>
          <dt className="detail-grid-wide">{t("partner.nextStep")}</dt>
          <dd className="detail-grid-wide">
            <PartnerNextStep companyId={companyId} partner={partner} />
          </dd>
          {partner.served_segments && partner.served_segments.length > 0 && (
            <>
              <dt>{t("partner.servedSegments")}</dt>
              <dd>{partner.served_segments.join(", ")}</dd>
            </>
          )}
        </dl>
      </PanelBody>
      {editing && (
        <PartnerForm
          companyId={companyId}
          partner={partner}
          intent="save"
          dialogTitle={t("partner.edit")}
          onCancel={() => setEditing(false)}
          onSaved={() => {
            setEditing(false);
            onSaved();
          }}
        />
      )}
    </Panel>
  );
}

// The Partner tab. A company is a partner when it has a `partner` row, so a 404
// from GET /companies/{id}/partner means "not a partner yet", not an error. It
// renders the setup form instead of the shared error state. Setup and edit
// both PUT the same UpsertPartnerRequest. The first creation carries no
// If-Match, since there is no prior version; an edit carries the partner's
// own `version`.
export function PartnerTab({
  companyId,
  programEnded,
}: Readonly<{ companyId: string; programEnded?: boolean }>) {
  const t = useT();
  const queryClient = useQueryClient();
  // Every partner write needs the company grant; the tier alone also needs
  // the partner grant, which the form asks for itself.
  const writeRefusal = useWriteRefusal("company", "update", {
    seat: t("settings.seatRefused"),
    grant: t("partner.writeRefused"),
  });
  const refusal = useMe().data?.authorization ? writeRefusal : undefined;
  const query = useQuery({
    queryKey: ["partner", companyId],
    queryFn: () => fetchPartner(companyId),
  });

  function invalidateAfterSave() {
    queryClient.invalidateQueries({ queryKey: ["partner", companyId] });
    queryClient.invalidateQueries({
      queryKey: ["company", companyId],
    });
    queryClient.invalidateQueries({ queryKey: ["companies"] });
    // The deal form asks this list whether a partner programme exists at all,
    // so making the first partner has to reach it. Otherwise its fields stay
    // absent there until the cache goes stale, and the setting looks lost.
    queryClient.invalidateQueries({ queryKey: ["partners"] });
  }

  return (
    <QueryGate query={query} pendingLabel={t("nav.partners")}>
      {(partner) => (
        <div className="record-stack">
          {partner ? (
            <PartnerDetail
              companyId={companyId}
              partner={partner}
              refusal={refusal}
              onSaved={invalidateAfterSave}
            />
          ) : (
            // Two bodies: the panel's own seam divides what there is none of
            // from how the first one is made.
            <Panel title={t("tab.partner")}>
              <PanelBody>
                <EmptyState>{t("partner.none")}</EmptyState>
              </PanelBody>
              <PanelBody>
                <SectionHeader title={t("partner.setup")} />
                {refusal ? (
                  <p>{refusal}</p>
                ) : (
                  <PartnerForm
                    companyId={companyId}
                    intent="create"
                    onSaved={invalidateAfterSave}
                  />
                )}
              </PanelBody>
            </Panel>
          )}
          {(partner || programEnded) && (
            <>
              <PartnerDeals companyId={companyId} />
              <PartnerCommissions companyId={companyId} />
            </>
          )}
        </div>
      )}
    </QueryGate>
  );
}
