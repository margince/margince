import { type ReactNode, lazy as reactLazy } from "react";
import type { CatalogLevel } from "./catalogs";
import { LocaleBoundary } from "./index";

export function lazy<Props extends object>(
  load: () => Promise<{ default: (props: Props) => ReactNode }>,
  level: CatalogLevel = "complete",
) {
  return reactLazy(async () => {
    const { default: View } = await load();
    return {
      default: (props: Props) => (
        <LocaleBoundary level={level}>
          <View {...props} />
        </LocaleBoundary>
      ),
    };
  });
}
