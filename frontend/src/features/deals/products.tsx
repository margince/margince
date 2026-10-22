import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { ArchiveAction } from "../../components/archive";
import { CreateAction, type CreateField } from "../../components/create";
import { EditAction } from "../../components/edit";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
} from "../../components/listquery";
import { Badge, OverflowMenu } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { KeyedName } from "../../design-system/keyedname";
import type { ListColumn } from "../../design-system/listtable";
import { sortState } from "../../design-system/listtable.query";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { formatMoney } from "../../format/format";
import { toMajorUnits, toMinorUnits } from "../../format/minorunits";
import { billingModelLabel } from "../../format/productbilling";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { billingOf, billingPatchOf } from "./billingclassification";
import "../lists/listsection.css";
import {
  currencyOptions,
  type LeadingCurrencies,
} from "../../format/currencychoices";
import { useLeadingCurrencies } from "../../lib/leadingcurrencies";
import {
  OTHER_UNIT,
  storedUnitOf,
  unitChoiceOf,
  unitLabel,
  unitOptions,
} from "./productunit";

type Product = components["schemas"]["Product"];

async function fetchProductsPage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<Product>> {
  const data = unwrap(
    await api.GET("/products", {
      params: {
        query: {
          q: query.q || undefined,
          sort: query.sort || undefined,
          include_archived: query.includeArchived || undefined,
          cursor: cursor || undefined,
          limit: listFetchLimit(query.perPage),
          ...query.filters,
        },
      },
    }),
  );
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
    },
  };
}

// Phase-3 line-item product picker reuses this list read, mapped to {id,name}.
export async function searchProductCandidates(
  q: string,
): Promise<{ id: string; name: string }[]> {
  const data = unwrap(
    await api.GET("/products", {
      params: { query: { q, limit: 10 } },
    }),
  );
  return data.data.map((p) => ({ id: p.id, name: p.name }));
}

// Built per render: option labels are shown as given, so they arrive translated.
function productFields(
  t: Translator,
  leadingCurrencies: LeadingCurrencies,
): CreateField[] {
  return [
    { key: "name", label: "product.name", required: true },
    { key: "sku", label: "product.sku" },
    { key: "description", label: "product.description" },
    {
      key: "unit",
      label: "product.unit",
      type: "select",
      required: true,
      options: unitOptions(t),
    },
    {
      key: "unit_other",
      label: "product.unitOtherLabel",
      required: true,
      showWhen: (values) => values.unit === OTHER_UNIT,
    },
    {
      key: "unit_price",
      label: "product.unitPrice",
      type: "number",
      required: true,
    },
    {
      key: "currency",
      label: "product.currency",
      type: "select",
      required: true,
      options: currencyOptions(leadingCurrencies),
    },
    { key: "default_tax_rate", label: "product.taxRate", type: "number" },
    {
      // The empty option is a real answer, and every product written before
      // this field existed carries it: nobody has said whether the price
      // repeats. It comes first, so a form left alone keeps saying that
      // instead of asserting a one-off price.
      key: "billing_model",
      label: "product.billingModel",
      type: "select",
      options: [
        { value: "", label: billingModelLabel(null, t) },
        { value: "one_time", label: billingModelLabel("one_time", t) },
        { value: "recurring", label: billingModelLabel("recurring", t) },
      ],
    },
    {
      // Only a recurring price has a period, and the server refuses one
      // without it, so the choice appears, and is owed, only then.
      key: "billing_interval_months",
      label: "product.billingInterval",
      type: "select",
      required: true,
      showWhen: (values) => values.billing_model === "recurring",
      options: BILLING_MONTHS.map((months) => ({
        value: String(months),
        label: t(CADENCE[months]),
      })),
    },
  ];
}

type BillingMonths = NonNullable<Product["billing_interval_months"]>;

const BILLING_MONTHS: readonly BillingMonths[] = [1, 3, 6, 12];

const CADENCE: Readonly<Record<BillingMonths, MessageKey>> = {
  1: "product.billingMonthly",
  3: "product.billingQuarterly",
  6: "product.billingHalfYearly",
  12: "product.billingYearly",
};

const PRICE_PER: Readonly<Record<BillingMonths, MessageKey>> = {
  1: "product.pricePerMonth",
  3: "product.pricePerQuarter",
  6: "product.pricePerHalfYear",
  12: "product.pricePerYear",
};

function billingLabel(p: Product, t: Translator): string {
  if (p.billing_model !== "recurring")
    return billingModelLabel(p.billing_model, t);
  return p.billing_interval_months
    ? t(CADENCE[p.billing_interval_months])
    : t("product.billingRecurring");
}

// The period under the amount, not after it: a 110px price column clips both.
function ProductPrice({ product }: Readonly<{ product: Product }>) {
  const t = useT();
  const { locale } = useLocale();
  const amount = formatMoney(
    product.unit_price_minor,
    product.currency,
    locale,
  );
  const months = product.billing_interval_months;
  if (product.billing_model !== "recurring" || !months) {
    return amount;
  }
  return (
    <CellStack>
      <span>{amount}</span>
      <span className="t-caption">{t(PRICE_PER[months])}</span>
    </CellStack>
  );
}

// Only a status that differs from the usual one has a name.
function productStatus(product: Product): MessageKey | null {
  if (product.archived_at) {
    return "product.archived";
  }
  return product.active ? null : "product.inactive";
}

function ProductStatus({ product }: Readonly<{ product: Product }>) {
  const t = useT();
  const status = productStatus(product);
  return status === null ? null : <Badge>{t(status)}</Badge>;
}

function editableProduct(p: Product) {
  const unit = unitChoiceOf(p.unit);
  return {
    ...p,
    unit: unit.choice,
    unit_other: unit.other,
    unit_price: String(toMajorUnits(p.unit_price_minor, p.currency)),
    billing_model: p.billing_model ?? "",
    billing_interval_months:
      p.billing_interval_months == null
        ? ""
        : String(p.billing_interval_months),
  };
}

// text narrows one value off the edit form's Record<string, unknown> without
// asserting: a field the reader left alone is absent, not an empty string, and
// an assertion would hand `undefined` to a scale that then silently defaults.
function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

// Major-unit price string -> integer minor units (P11: no float money on the
// wire), at the scale the PRODUCT's own currency carries. A hard-coded hundred
// priced a yen product at a hundredth and a dinar product at ten times.
function toMinor(major: string | undefined, currency: string): number {
  return toMinorUnits(Number(major ?? "0"), currency);
}

/**
 * The priced things an offer line can point at, as a section of Settings → Data
 * model.
 *
 * Not a screen of its own: it was one, reached by a card that existed only to
 * send you here, and a door is not a section. So it renders no `.wrap` — the
 * settings page owns the reading column — and names itself, since the shell's
 * page title now belongs to the whole data-model page.
 *
 * A Panel around the list surface rather than a bare heading above it: the two
 * surfaces beside it on this tab are cards, and a section whose name floats on
 * the page ground while its neighbours' names sit inside their cards reads as
 * two different kinds of thing on one page.
 */
export function ProductsAdmin() {
  const t = useT();
  // One grant per affordance, each named for the request it actually issues:
  // New POSTs, Edit PATCHes, Archive DELETEs. `useCanWrite` rather than `useCan`
  // because all three mutate, and the licensing seat is clamped on the HTTP
  // method before RBAC is consulted — a read seat holding product:update would
  // otherwise be handed an editor every save bounces off.
  const me = useMe();
  const canCreate = useCanWrite("product", "create");
  const canUpdate = useCanWrite("product", "update");
  const canArchive = useCanWrite("product", "delete");
  // Scoped: this table and the offer-template table are drawn together on the
  // settings Data-model tab, so one flat parameter space described both at once
  // — sorting products by `sku` sent `sort=sku` to `/offer-templates`, and the
  // `active` chip narrowed the templates read as well.
  const list = useListQuery<Product>({
    key: "products",
    fetchPage: fetchProductsPage,
    initialSort: "name",
    paramScope: "products",
  });
  const leading = useLeadingCurrencies();
  const fields = productFields(t, leading);

  const createProduct = async (values: Record<string, string>) => {
    return unwrap(
      await api.POST("/products", {
        body: {
          name: (values.name ?? "").trim(),
          sku: values.sku?.trim() || null,
          description: values.description?.trim() || null,
          unit:
            storedUnitOf(values.unit ?? "", values.unit_other ?? "") || null,
          unit_price_minor: toMinor(
            values.unit_price,
            values.currency || leading[0],
          ),
          currency: values.currency || leading[0],
          default_tax_rate: values.default_tax_rate
            ? Number(values.default_tax_rate)
            : null,
          ...billingOf(values),
          source: "manual",
        },
      }),
    );
  };

  // `sku`/`description` are nullable contract fields, so an emptied form
  // input sends an explicit `null` to clear the stored value. `unit`/
  // `currency` are non-nullable, so an emptied input instead sends
  // `undefined` (omitted from the PATCH body) to leave the existing value
  // unchanged rather than overwrite it with an invalid empty string.
  const updateProduct =
    (product: Product) => async (values: Record<string, unknown>) => {
      return unwrap(
        await api.PATCH("/products/{id}", {
          params: {
            path: { id: product.id },
            ...ifMatch(requireVersion(product.version)),
          },
          body: {
            name: String(values.name).trim(),
            sku: (values.sku as string)?.trim() || null,
            description: (values.description as string)?.trim() || null,
            unit:
              storedUnitOf(text(values.unit), text(values.unit_other)) ||
              undefined,
            // The form's currency, else the product's stored one: the same value the
            // PATCH below sends, so the amount and its scale cannot disagree. Narrowed,
            // not asserted: `as string` on an undefined value would silently scale the
            // price at the two-digit default.
            unit_price_minor: toMinor(
              text(values.unit_price),
              text(values.currency) || product.currency,
            ),
            currency: text(values.currency) || undefined,
            default_tax_rate: values.default_tax_rate
              ? Number(values.default_tax_rate)
              : undefined,
            ...billingPatchOf(values),
          },
        }),
      );
    };

  // Only while some row has a status to show, or the list is ordered by it.
  const statusColumn: ListColumn<Product> = {
    key: "status",
    header: t("product.status"),
    sort: "active",
    cell: (p: Product) => <ProductStatus product={p} />,
  };

  // The per-row affordances, in a column that exists only while at least one of
  // them does. Keeping the column and emptying its cells would leave a headed,
  // width-taking, column-picker-listed strip of nothing, which reads as a table
  // that lost its buttons rather than as a permission — so the column goes,
  // exactly as the custom-fields table drops its own.
  const rowMenu: ListColumn<Product> = {
    key: "actions",
    header: t("table.actions"),
    verbs: "menu",
    cell: (p: Product) => (
      <span className="cell-actions">
        <OverflowMenu label={t("table.rowActions", { name: p.name })}>
          {canUpdate && (
            <EditAction
              labelled
              label={t("product.edit")}
              savedMessage={(saved) =>
                t("record.saveDone", { name: saved.name })
              }
              invalidate="products"
              recordKey="product"
              record={editableProduct(p)}
              update={updateProduct(p)}
              fields={fields}
            />
          )}
          {canArchive && (
            <ArchiveAction
              label={t("product.archive")}
              confirmText={t("product.archiveConfirm")}
              archivedMessage={t("record.archiveDone", { name: p.name })}
              invalidate="products"
              recordKey="product"
              onArchived={() => list.refetch()}
              archive={async () => {
                const data = unwrap(
                  await api.DELETE("/products/{id}", {
                    params: {
                      path: { id: p.id },
                      ...ifMatch(requireVersion(p.version)),
                    },
                  }),
                );
                return data ?? p;
              }}
            />
          )}
        </OverflowMenu>
      </span>
    ),
  };

  return (
    <Panel
      className="listsection"
      title={t("product.title")}
      titleAction={
        canCreate ? (
          <CreateAction
            label={t("product.new")}
            invalidate="products"
            // `stay`: a fresh product has no page of its own to open. The list
            // it joins is already on screen.
            screen="settings"
            stay
            create={createProduct}
            fields={fields}
          />
        ) : undefined
      }
    >
      <PanelBody className="listsection-intro">
        <PanelIntro>{t("product.settingsSub")}</PanelIntro>
        {/* A reader who holds no write verb here sees a list with no editor, and
            silence about why is a claim that the list has none. The posture is
            stated ONCE for the whole section (design-system README, "Absent,
            disabled, or withheld"); a reader who can edit but not archive needs no
            page-level notice, because the affordances they do hold say what they
            may do. */}
        {/* me.isSuccess first: every capability hook fails CLOSED while the
            probe is in flight, so branching on the grants alone flashes a
            read-only notice at the admin who holds all three. Gate on the
            probe, not on its absence. */}
        {me.isSuccess && !canCreate && !canUpdate && !canArchive && (
          <PanelIntro>{t("product.readOnly")}</PanelIntro>
        )}
      </PanelBody>
      <ListTable
        state={list}
        hideShortChrome
        unit="unit.products"
        columns={[
          {
            key: "name",
            header: t("product.name"),
            sort: "name",
            cell: (p: Product) =>
              p.sku ? <KeyedName name={p.name} code={p.sku} /> : p.name,
            fixed: true,
          },
          {
            key: "price",
            header: t("product.unitPrice"),
            sort: "unit_price_minor",
            cell: (p: Product) => <ProductPrice product={p} />,
            numeric: true,
          },
          {
            key: "unit",
            header: t("product.unit"),
            cell: (p: Product) => unitLabel(p.unit, t),
          },
          {
            key: "billing",
            header: t("product.billingModel"),
            cell: (p: Product) => billingLabel(p, t),
          },
          ...(list.rows.some((p) => productStatus(p) !== null) ||
          sortState(statusColumn, list.query.sort) !== null
            ? [statusColumn]
            : []),
          ...(canUpdate || canArchive ? [rowMenu] : []),
        ]}
        rowKey={(p) => p.id}
        chips={[
          {
            key: "active",
            label: "product.activeFilter",
            allLabel: "product.activeFilterAll",
            options: [{ value: "true", label: "product.active" }],
          },
        ]}
      />
    </Panel>
  );
}
