// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Badge, EmptyState } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Checkbox, Field } from "../../design-system/field";
import { Select } from "../../design-system/select";
import { SetPicker } from "../../design-system/setpicker";
import { countryOptions } from "../../format/assignedcountries";
import { formatNumber } from "../../format/format";
import { INDUSTRY_CODES, industryLabelKey } from "../../format/industry";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useProductOptions } from "./reporting.dimensions";
import {
  type MemberRef,
  type Members,
  predicateOf,
  type Rule,
} from "./reporting.initiativegoal";

export function MemberFields({
  rule,
  onRule,
  members,
  pending,
  picked,
  onPick,
  excludedSet,
  onToggle,
  includeFuture,
  onIncludeFuture,
}: Readonly<{
  rule: Rule;
  onRule: (rule: Rule) => void;
  members: Members | undefined;
  pending: boolean;
  picked: MemberRef[];
  onPick: (picked: MemberRef[]) => void;
  excludedSet: ReadonlySet<string>;
  onToggle: (member: Members["data"][number], included: boolean) => void;
  includeFuture: boolean;
  onIncludeFuture: (value: boolean) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const products = useProductOptions();
  const candidates = useQuery({
    queryKey: ["initiative-candidates"],
    queryFn: async () => {
      const [deals, leads] = await Promise.all([
        api.GET("/deals", {
          params: { query: { limit: 200, status: "open" } },
        }),
        api.GET("/leads", { params: { query: { limit: 200 } } }),
      ]);
      return {
        deals: unwrap(deals).data.map((deal) => ({
          value: deal.id,
          label: deal.name,
        })),
        leads: unwrap(leads).data.map((lead) => ({
          value: lead.id,
          label: lead.full_name || lead.company_name || lead.id,
        })),
      };
    },
    staleTime: 60_000,
  });
  const pickedOf = (type: "lead" | "deal") =>
    picked
      .filter((ref) => ref.record_type === type)
      .map((ref) => ref.record_id);
  const setPickedOf = (type: "lead" | "deal", ids: string[]) =>
    onPick([
      ...picked.filter((ref) => ref.record_type !== type),
      ...ids.map((record_id) => ({ record_type: type, record_id })),
    ]);
  const pickerWords = {
    removeLabel: (item: { label: string }) =>
      t("reporting.removePick", { name: item.label }),
    none: t("reporting.nonePicked"),
    addLabel: t("reporting.addRecord"),
    searchLabel: t("reporting.searchRecords"),
    noMatch: t("reporting.noMatchingRecords"),
    doneLabel: t("reporting.pickDone"),
  };
  return (
    <div className="form-stack">
      <p>{t("reporting.membersPurpose")}</p>
      <Field label={t("reporting.product")}>
        {(field) => (
          <Select
            {...field}
            value={rule.product}
            options={[
              { value: "", label: t("reporting.anyProduct") },
              ...(products.data ?? []),
            ]}
            onChange={(product) => onRule({ ...rule, product })}
          />
        )}
      </Field>
      <Field label={t("reporting.industry")}>
        {(field) => (
          <Select
            {...field}
            value={rule.industry}
            options={[
              { value: "", label: t("reporting.anyIndustry") },
              ...INDUSTRY_CODES.map((code) => ({
                value: code,
                label: industryLabelKey(code)
                  ? t(industryLabelKey(code) ?? "reporting.industry")
                  : code,
              })),
            ]}
            onChange={(industry) => onRule({ ...rule, industry })}
          />
        )}
      </Field>
      <Field label={t("reporting.country")}>
        {(field) => (
          <Select
            {...field}
            value={rule.country}
            options={[
              { value: "", label: t("reporting.anyCountry") },
              ...countryOptions(locale),
            ]}
            onChange={(country) => onRule({ ...rule, country })}
          />
        )}
      </Field>
      <p className="t-caption">
        {predicateOf(rule, "deal")
          ? t("reporting.ruleAnd")
          : t("reporting.noConditions")}
      </p>
      <Checkbox
        label={t("reporting.includeFuture")}
        checked={includeFuture}
        onChange={(event) => onIncludeFuture(event.target.checked)}
      />
      <SetPicker
        legend={t("reporting.pickDeals")}
        options={candidates.data?.deals ?? []}
        values={pickedOf("deal")}
        onChange={(ids) => setPickedOf("deal", ids)}
        {...pickerWords}
      />
      <SetPicker
        legend={t("reporting.pickLeads")}
        options={candidates.data?.leads ?? []}
        values={pickedOf("lead")}
        onChange={(ids) => setPickedOf("lead", ids)}
        {...pickerWords}
      />
      <p className="t-caption">{t("reporting.pickExplains")}</p>
      {members && (
        <>
          <p>
            {t("reporting.memberCounts", {
              leads: formatNumber(members.leads, locale),
              deals: formatNumber(members.deals, locale),
              companies: formatNumber(members.companies, locale),
            })}
            {members.truncated && ` ${t("reporting.membersTruncated")}`}
          </p>
          <MembersTable
            members={members}
            excludedSet={excludedSet}
            onToggle={onToggle}
          />
        </>
      )}
      {pending && <p className="t-caption">{t("reporting.membersPending")}</p>}
      {!members && !pending && (
        <EmptyState>{t("reporting.saveGoalFirst")}</EmptyState>
      )}
    </div>
  );
}

export function MembersTable({
  members,
  excludedSet,
  onToggle,
}: Readonly<{
  members: Members;
  excludedSet: ReadonlySet<string>;
  onToggle: (member: Members["data"][number], included: boolean) => void;
}>) {
  const t = useT();
  return (
    <DataTable
      label={t("reporting.stepMembers")}
      rows={members.data}
      rowKey={(member) => `${member.record_type}:${member.record_id}`}
      columns={[
        {
          key: "include",
          header: t("reporting.include"),
          render: (member) => (
            <Checkbox
              label={t("reporting.includeRecord", { name: member.label })}
              checked={member.included && !excludedSet.has(member.record_id)}
              onChange={(event) => onToggle(member, event.target.checked)}
            />
          ),
        },
        {
          key: "record",
          header: t("reporting.memberRecord"),
          render: (member) => (
            <>
              {member.label}
              <p className="t-sub">
                {t(`reporting.recordType.${member.record_type}`)}
                {member.company_label ? ` · ${member.company_label}` : ""}
              </p>
            </>
          ),
        },
        {
          key: "reason",
          header: t("reporting.whyIncluded"),
          render: (member) => (
            <Badge>{t(`reporting.memberReason.${member.reason}`)}</Badge>
          ),
        },
      ]}
    />
  );
}
