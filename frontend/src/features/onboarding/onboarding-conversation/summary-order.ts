/**
 * The order of one voice step's corpus writes, and which summary the meter
 * may still take. Writes settle out of order, and only the newest by issue
 * order may move the meter. An ingest issued before a removal that landed may
 * still count the removed sample, so it never outranks that removal.
 */
export class SummaryOrder {
  private issued = 0;
  private applied = 0;
  private floor = 0;

  /** Stamps a write as it is issued. */
  issue(): number {
    this.issued += 1;
    return this.issued;
  }

  /** Records that the removal stamped `seq` landed at the server. */
  removed(seq: number): void {
    this.floor = Math.max(this.floor, seq);
  }

  /** Takes write `seq`'s summary when it is the newest the meter may show. */
  take(seq: number): boolean {
    if (seq <= this.applied || seq < this.floor) {
      return false;
    }
    this.applied = seq;
    return true;
  }
}
