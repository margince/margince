/** Nearest-rank quantile, the method the Go harness reports, so a value is a
 * latency that happened rather than an interpolation between two. Every
 * frontend benchmark uses this one spelling. */
export function nearestRank(samples: number[], quantile: number): number {
  const sorted = [...samples].sort((a, b) => a - b);
  const rank = Math.ceil(sorted.length * quantile) - 1;
  return sorted[Math.min(Math.max(rank, 0), sorted.length - 1)];
}
