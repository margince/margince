import { type ComponentType, type ReactNode, lazy as reactLazy } from "react";
import type { CatalogLevel } from "./catalogs";
import { LocaleBoundary } from "./index";

export function lazy<Props extends object>(
  load: () => Promise<{ default: (props: Props) => ReactNode }>,
  level: CatalogLevel = "complete",
  Boundary?: ComponentType<{ children: ReactNode }>,
) {
  const Deferred = reactLazy(async () => {
    const { default: View } = await load();
    return {
      default: (props: Props) => (
        <LocaleBoundary level={level}>
          <View {...props} />
        </LocaleBoundary>
      ),
    };
  });
  if (!Boundary) return Deferred;
  return function DeferredView(props: Props) {
    return (
      <Boundary>
        <Deferred {...props} />
      </Boundary>
    );
  };
}
