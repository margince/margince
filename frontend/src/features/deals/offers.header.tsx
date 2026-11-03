import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import { Heading } from "../../design-system/heading";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useLegalEntities } from "../../lib/legalentities.queries";
import { problemMessageOf, unwrap } from "../../lib/problem";
import type { Offer, OfferTemplate } from "./offers";
import { OfferCurrencyField } from "./offers.currencyfield";

async function searchCompanyCandidates(
  q: string,
): Promise<RecordPickerCandidate[]> {
  const data = unwrap(
    await api.GET("/companies", {
      params: { query: { q, limit: 10 } },
    }),
  );
  return data.data.map((company) => ({
    id: company.id,
    name: company.display_name,
  }));
}

function useOfferTemplates() {
  return useQuery({
    queryKey: ["offer-templates-all"],
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/offer-templates", {
          params: { query: { limit: 100 } },
        }),
      );
      return data.data;
    },
  });
}

type HeaderEditValues = {
  currency: string;
  buyer_company_id: string | null;
  valid_until: string;
  template_id: string | null;
  legal_entity_id: string | null;
  intro_text: string;
  terms_text: string;
};

// RecordPicker only highlights a selection among candidates its own search
// turned up. It cannot preview a value set outside its session. A reopened
// header-edit modal has only `offer.buyer_company_id` (a bare id), so the
// picker would render empty though a buyer company is set. This resolves that
// id to a name (the GET /companies/{id} lookup entityref.tsx uses too). It
// prefers the caller's override, set once the reader picks another company.
function useBuyerCompanyPreview(buyerCompanyId: string | null, open: boolean) {
  const [override, setOverride] = useState<RecordPickerCandidate | null>(null);
  const existingQuery = useQuery({
    queryKey: ["company", "ref", buyerCompanyId],
    queryFn: buyerCompanyId
      ? async () => {
          const data = unwrap(
            await api.GET("/companies/{id}", {
              params: { path: { id: buyerCompanyId } },
            }),
          );
          return {
            id: buyerCompanyId,
            name: data.display_name ?? "",
          } satisfies RecordPickerCandidate;
        }
      : skipToken,
    enabled: open,
    staleTime: 60_000,
  });
  const buyerCompany =
    override ?? (buyerCompanyId ? (existingQuery.data ?? null) : null);
  return { buyerCompany, setBuyerCompanyOverride: setOverride };
}

export function EditOfferHeaderModal({
  open,
  onClose,
  offer,
}: Readonly<{ open: boolean; onClose: () => void; offer: Offer }>) {
  const t = useT();
  const headingId = useId();
  const queryClient = useQueryClient();
  const templatesQuery = useOfferTemplates();
  const [values, setValues] = useState<HeaderEditValues>({
    currency: offer.currency,
    buyer_company_id: offer.buyer_company_id ?? null,
    valid_until: offer.valid_until ?? "",
    template_id: offer.template_id ?? null,
    legal_entity_id: offer.legal_entity_id ?? null,
    intro_text: offer.intro_text ?? "",
    terms_text: offer.terms_text ?? "",
  });
  const { buyerCompany, setBuyerCompanyOverride } = useBuyerCompanyPreview(
    offer.buyer_company_id ?? null,
    open,
  );
  // Only the closed→open transition reprimes the form. A background refetch
  // handing this component a fresh `offer` mid-edit must never clobber what the
  // reader is typing (same convention as EditRecordModal, edit.tsx).
  const wasOpen = useRef(false);
  useEffect(() => {
    if (open && !wasOpen.current) {
      setValues({
        currency: offer.currency,
        buyer_company_id: offer.buyer_company_id ?? null,
        valid_until: offer.valid_until ?? "",
        template_id: offer.template_id ?? null,
        legal_entity_id: offer.legal_entity_id ?? null,
        intro_text: offer.intro_text ?? "",
        terms_text: offer.terms_text ?? "",
      });
      setBuyerCompanyOverride(null);
    }
    wasOpen.current = open;
  }, [open, offer, setBuyerCompanyOverride]);

  const mutation = useMutation({
    mutationFn: async (input: HeaderEditValues) => {
      return unwrap(
        await api.PATCH("/offers/{id}", {
          params: {
            path: { id: offer.id },
            ...ifMatch(requireVersion(offer.version)),
          },
          body: {
            currency: input.currency,
            buyer_company_id: input.buyer_company_id,
            valid_until: input.valid_until || null,
            template_id: input.template_id,
            // Sent only when the reader picked one: an offer from before
            // offers named an entity keeps naming none until somebody does.
            ...(input.legal_entity_id
              ? { legal_entity_id: input.legal_entity_id }
              : {}),
            intro_text: input.intro_text || null,
            terms_text: input.terms_text || null,
          },
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["offer", offer.id], data);
      onClose();
    },
  });

  const errorMessage = mutation.isError
    ? problemMessageOf(mutation.error, t)
    : null;

  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("offer.edit")}
      </Heading>
      <div className="form-stack">
        <OfferCurrencyField
          value={values.currency}
          lineItems={offer.line_items}
          onChange={(currency) => setValues((prev) => ({ ...prev, currency }))}
        />
        <Field label={t("offer.validUntil")}>
          {(control) => (
            <DateInput
              {...control}
              value={isoOrBlank(values.valid_until)}
              onChange={(event) =>
                setValues((prev) => ({
                  ...prev,
                  valid_until: event.target.value,
                }))
              }
            />
          )}
        </Field>
        <div className="field">
          <span className="t-label">{t("offer.buyerCompany")}</span>
          <RecordPicker
            label={t("offer.buyerCompany")}
            searchTargets={searchCompanyCandidates}
            selected={buyerCompany}
            onPick={(candidate) => {
              setBuyerCompanyOverride(candidate);
              setValues((prev) => ({
                ...prev,
                buyer_company_id: candidate.id,
              }));
            }}
          />
          {buyerCompany && (
            <p className="t-caption">
              {t("offer.buyerCompanyConfirm", { name: buyerCompany.name })}
            </p>
          )}
        </div>
        <Field label={t("offer.template")}>
          {(control) => (
            <Select
              {...control}
              value={values.template_id ?? ""}
              onChange={(value) =>
                setValues((prev) => ({ ...prev, template_id: value || null }))
              }
              // The dash is a real option for "no template", not the select's
              // placeholder. A placeholder only stands in for an unset value,
              // and an offer that picked a template must be able to drop it.
              options={[
                { value: "", label: "—" },
                ...(templatesQuery.data ?? []).map(
                  (template: OfferTemplate) => ({
                    value: template.id,
                    label: template.name,
                  }),
                ),
              ]}
            />
          )}
        </Field>
        <OfferEntityField
          value={values.legal_entity_id}
          onChange={(legal_entity_id) =>
            setValues((prev) => ({ ...prev, legal_entity_id }))
          }
        />
        <Field label={t("offer.introText")}>
          {(control) => (
            <TextInput
              {...control}
              value={values.intro_text}
              onChange={(event) =>
                setValues((prev) => ({
                  ...prev,
                  intro_text: event.target.value,
                }))
              }
            />
          )}
        </Field>
        <Field label={t("offer.termsText")}>
          {(control) => (
            <TextInput
              {...control}
              value={values.terms_text}
              onChange={(event) =>
                setValues((prev) => ({
                  ...prev,
                  terms_text: event.target.value,
                }))
              }
            />
          )}
        </Field>
        {errorMessage && <ErrorLine>{errorMessage}</ErrorLine>}
      </div>
      <div className="actions">
        <Button onClick={onClose}>{t("deals.cancel")}</Button>
        <Button
          variant="primary"
          disabled={mutation.isPending}
          onClick={(event) => {
            if (!refuseUnreadableField(event.currentTarget)) {
              mutation.mutate(values);
            }
          }}
        >
          {t("record.save")}
        </Button>
      </div>
    </Modal>
  );
}

// Which legal entity issues the offer. Shown only while the installation
// trades as more than one: with one, there is nothing to choose.
function OfferEntityField({
  value,
  onChange,
}: Readonly<{ value: string | null; onChange: (id: string) => void }>) {
  const t = useT();
  const entities = useLegalEntities().data ?? [];
  if (entities.length < 2) {
    return null;
  }
  const fallback = entities.find((entity) => entity.is_default);
  return (
    <Field label={t("offer.legalEntity")} hint={t("offer.legalEntityHint")}>
      {(control) => (
        <Select
          {...control}
          value={value ?? fallback?.id ?? ""}
          onChange={onChange}
          options={entities.map((entity) => ({
            value: entity.id,
            label: entity.display_name || t("legalEntities.defaultUnnamed"),
          }))}
        />
      )}
    </Field>
  );
}
