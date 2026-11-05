// The cache key of the project page's composite read. Under the
// ["project", id] prefix, so the edit and archive actions refresh it too.
export function project360Key(projectId: string) {
  return ["project", projectId, "360"] as const;
}
