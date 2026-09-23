import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { searchCompanyCandidates } from "./contactemployers";
import { invalidateRecord } from "./recordwritekeys";

type Item = components["schemas"]["EmploymentImportItem"];
type Request = components["schemas"]["EmploymentImportRequest"];
type Contact360 = components["schemas"]["Contact360"];

export function ImportedEmploymentHistory({
  view,
  canEdit,
}: Readonly<{ view: Contact360; canEdit: boolean }>) {
  const t = useT();
  const client = useQueryClient();
  const id = view.contact.id;
  const hasEvidence =
    view.provider_profiles?.some(
      (profile) => profile.current_employment || profile.job_history.length > 0,
    ) ?? false;
  const key = ["employmentImport", id];
  const reading = useQuery({
    queryKey: key,
    enabled: hasEvidence,
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/contacts/{id}/employment-import",
        { params: { path: { id } } },
      );
      if (error) throwProblem(error);
      return data;
    },
  });
  const [resolving, setResolving] = useState<Item | null>(null);
  const apply = useMutation({
    mutationFn: async ({
      contactId,
      body,
    }: {
      contactId: string;
      body: Request;
    }) => {
      const { data, error } = await api.POST(
        "/contacts/{id}/employment-import",
        { params: { path: { id: contactId } }, body },
      );
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: async (report) => {
      client.setQueryData(["employmentImport", report.contact_id], report);
      await invalidateRecord(client, "contact", report.contact_id);
      await client.invalidateQueries({ queryKey: ["companies"] });
      await client.invalidateQueries({
        queryKey: ["contactEmployments", report.contact_id],
      });
      setResolving(null);
    },
  });
  if (!hasEvidence) return null;
  const linkedIds = new Set(
    view.employments?.data.map((role) => role.relationship_id),
  );
  const outstanding = (reading.data?.items ?? []).filter(
    (item) =>
      item.state !== "dismissed" &&
      (!item.relationship_id || !linkedIds.has(item.relationship_id)),
  );
  const groups = new Map<string, Item[]>();
  for (const item of outstanding) {
    const company = item.company_id ?? item.company_name.toLowerCase();
    groups.set(company, [...(groups.get(company) ?? []), item]);
  }
  return (
    <div className="form-stack">
      {/* A read's warnings are a fact of that read, not news. */}
      {reading.data?.warnings?.map((warning) => (
        <ErrorLine standing key={warning}>
          {warning}
        </ErrorLine>
      ))}
      {reading.isPending && <p>{t("employment.importLoading")}</p>}
      <ErrorLine error={reading.error} />
      {canEdit && outstanding.some((item) => item.state === "pending") && (
        <Button
          disabled={apply.isPending}
          onClick={() =>
            apply.mutate({
              contactId: id,
              body: { action: "apply", resolve_group: false },
            })
          }
        >
          {t("employment.apply")}
        </Button>
      )}
      {[...groups.entries()].map(([group, items]) => (
        <div key={group} className="form-stack">
          {items[0]?.company_id ? (
            <Button
              variant="ghost"
              onClick={() => {
                const company = items[0]?.company_id;
                if (company) navigate({ screen: "companies", id: company });
              }}
            >
              {items[0].company_name}
            </Button>
          ) : (
            <strong>{items[0]?.company_name}</strong>
          )}
          {items.map((item) => (
            <div key={item.key} className="form-stack">
              <span>{item.role || t("field.unset")}</span>
              <span className="t-caption">
                {[item.started, item.ended].filter(Boolean).join(" – ")} ·{" "}
                {t(`employment.status.${item.employment_status}`)} ·{" "}
                {item.provider}
              </span>
              {item.state !== "linked" && (
                <span>
                  {t(
                    item.state === "needs_review"
                      ? "employment.review"
                      : "employment.matchNeeded",
                  )}
                </span>
              )}
              {canEdit && item.state !== "linked" && (
                <div className="card-actions">
                  <Button
                    disabled={apply.isPending}
                    onClick={() => {
                      apply.reset();
                      setResolving(item);
                    }}
                  >
                    {t("employment.resolve")}
                  </Button>
                  <Button
                    disabled={apply.isPending}
                    onClick={() =>
                      apply.mutate({
                        contactId: id,
                        body: {
                          action: "dismiss",
                          key: item.key,
                          resolve_group: false,
                        },
                      })
                    }
                  >
                    {t("employment.dismiss")}
                  </Button>
                </div>
              )}
            </div>
          ))}
        </div>
      ))}
      {reading.data?.items
        .filter((item) => item.state === "linked" && item.research_state)
        .map(
          (item, index, items) =>
            items.findIndex((peer) => peer.company_id === item.company_id) ===
              index && (
              <p key={item.key}>
                <Button
                  variant="ghost"
                  onClick={() => {
                    if (item.company_id)
                      navigate({ screen: "companies", id: item.company_id });
                  }}
                >
                  {item.company_name}
                </Button>{" "}
                · {t("employment.research")}:{" "}
                {item.research_state === "awaiting_research_policy"
                  ? t("employment.researchDeferred")
                  : item.research_state === "needs_website"
                    ? t("employment.researchNeedsWebsite")
                    : researchLabel(item.research_state ?? "", t)}
              </p>
            ),
        )}
      {!resolving && <ErrorLine error={apply.error} />}
      {resolving && (
        <EmploymentMatchModal
          key={resolving.key}
          item={resolving}
          pending={apply.isPending}
          error={apply.error}
          onClose={() => setResolving(null)}
          onSave={(body) => apply.mutate({ contactId: id, body })}
        />
      )}
    </div>
  );
}

function EmploymentMatchModal({
  item,
  pending,
  error,
  onClose,
  onSave,
}: Readonly<{
  item: Item;
  pending: boolean;
  error: unknown;
  onClose: () => void;
  onSave: (body: Request) => void;
}>) {
  const t = useT();
  const heading = useId();
  const [company, setCompany] = useState<RecordPickerCandidate | null>(null);
  const [domain, setDomain] = useState(item.domain ?? "");
  const [started, setStarted] = useState(item.started ?? "");
  const [ended, setEnded] = useState(item.ended ?? "");
  const [status, setStatus] = useState<Item["employment_status"]>(
    item.employment_status,
  );
  return (
    <Modal open onClose={onClose} labelledBy={heading}>
      <Heading
        size="large"
        id={heading}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {t("employment.resolve")}
      </Heading>
      <div className="form-stack">
        <p>
          {item.company_name} · {item.role}
        </p>
        <RecordPicker
          label={t("contact.rail.employer")}
          searchTargets={searchCompanyCandidates}
          selected={company}
          onPick={setCompany}
          disabled={pending}
        />
        {!company && (
          <Field
            label={t("employment.website")}
            hint={t("employment.websiteHint")}
          >
            {(control) => (
              <TextInput
                {...control}
                value={domain}
                onChange={(event) => setDomain(event.target.value)}
                disabled={pending}
              />
            )}
          </Field>
        )}
        <Field label={t("employment.start")} hint={t("employment.dateHint")}>
          {(control) => (
            <TextInput
              {...control}
              value={started}
              onChange={(e) => setStarted(e.target.value)}
              disabled={pending}
            />
          )}
        </Field>
        <Field label={t("employment.end")} hint={t("employment.dateHint")}>
          {(control) => (
            <TextInput
              {...control}
              value={ended}
              onChange={(e) => setEnded(e.target.value)}
              disabled={pending}
            />
          )}
        </Field>
        <Select
          aria-label={t("employment.statusLabel")}
          value={status}
          onChange={(value) => {
            if (
              value === "current" ||
              value === "former" ||
              value === "unknown"
            )
              setStatus(value);
          }}
          options={(["current", "former", "unknown"] as const).map((value) => ({
            value,
            label: t(`employment.status.${value}`),
          }))}
          disabled={pending}
        />
        <ErrorLine error={error} />
        <Button
          disabled={
            pending ||
            (!company && !domain.trim()) ||
            !/^$|^\d{4}-\d{2}(-\d{2})?$/.test(started) ||
            !/^$|^\d{4}-\d{2}(-\d{2})?$/.test(ended)
          }
          onClick={() =>
            onSave({
              action: "resolve",
              resolve_group: true,
              key: item.key,
              company_id: company?.id,
              domain: company ? undefined : domain.trim(),
              employment_status: status,
              started,
              ended,
            })
          }
        >
          {t("employment.saveMatch")}
        </Button>
      </div>
    </Modal>
  );
}

function researchLabel(state: string, t: ReturnType<typeof useT>): string {
  switch (state) {
    case "queued":
      return t("employment.research.queued");
    case "running":
      return t("employment.research.running");
    case "done":
      return t("employment.research.done");
    case "partial":
      return t("employment.research.partial");
    case "failed":
      return t("employment.research.failed");
    case "cancelled":
      return t("employment.research.cancelled");
    case "deferred":
      return t("employment.researchDeferred");
    default:
      return t("employment.researchQueued");
  }
}
