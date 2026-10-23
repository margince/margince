import { useState } from "react";
import { Button, Field, SearchField } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { useDeals } from "../lib/deals.queries";

export function CompanyDealPicker({
  companyId,
  value,
  onChange,
}: Readonly<{
  companyId: string;
  value: string;
  onChange: (id: string) => void;
}>) {
  const t = useT();
  const [search, setSearch] = useState("");
  const deals = useDeals({
    pipelineId: "",
    q: search,
    sort: "",
    includeArchived: false,
    filters: { company_id: companyId },
  });
  const options =
    deals.data?.pages.flatMap((page) =>
      page.data.map((deal) => ({ value: deal.id, label: deal.name })),
    ) ?? [];
  return (
    <>
      <SearchField
        aria-label={t("dealSuggestion.searchExisting")}
        value={search}
        onChange={(event) => {
          setSearch(event.target.value);
          onChange("");
        }}
      />
      <Field label={t("dealSuggestion.existingDeal")} required>
        {(control) => (
          <Select
            {...control}
            value={value}
            options={options}
            onChange={onChange}
            disabled={deals.isPending}
          />
        )}
      </Field>
      {deals.hasNextPage && (
        <Button
          pending={deals.isFetchingNextPage}
          onClick={() => deals.fetchNextPage()}
        >
          {t("dealSuggestion.moreDeals")}
        </Button>
      )}
      {!deals.isPending && !deals.isError && options.length === 0 && (
        <p className="sub">{t("dealSuggestion.noMatchingDeals")}</p>
      )}
      <ErrorLine error={deals.error} />
    </>
  );
}
