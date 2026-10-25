import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import { unwrap } from "./problem";

// The company types installed extensions add. The generated types carry no enum
// for them, so the server answers instead. It changes only on deploy, so it is
// fetched once.
export function useCompanyTypeDirectory() {
  return useQuery({
    queryKey: ["company-types"],
    queryFn: async () => unwrap(await api.GET("/company-types")),
    staleTime: Number.POSITIVE_INFINITY,
  });
}

// What a screen prints for an extension's type: the declared label. A company
// can still carry a type whose unit was removed, so that one prints the type's
// own name and a reader never sees `ext:<unit>:`.
export function companyTypeLabel(
  value: string,
  directory: readonly { value: string; label: string }[],
): string {
  const declared = directory.find((entry) => entry.value === value);
  if (declared) {
    return declared.label;
  }
  const type = value.split(":").at(-1) ?? value;
  return type.replaceAll("_", " ");
}
