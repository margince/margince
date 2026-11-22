<!-- prose:plain -->
<!-- describes: frontend/src/features/brief/brief.payments.tsx frontend/src/features/brief/brief.payments.queries.ts backend/internal/modules/finance -->
# Customer payments

**Customer payments** shows overdue invoices for the companies whose payment data you may read. You need both company and finance read access. The card shows the companies you own or that someone shared with you, not those a teammate owns. An unshared account outside that scope adds nothing to the amounts, counts or invoice list.

Each currency has its own overdue total. The card names up to three customers, starting with the oldest overdue invoice. **View overdue invoices** opens the invoice list. Open a customer there to see its **Finance** tab. The date under the card says when the accounting data was last read.

With no accounting source connected, or no linked company you may read, the card stays off Home. After a complete sync with no overdue invoices, it says **No overdue invoices**. While a first sync or a new customer link still waits for data, amounts stay hidden. If a later sync fails, the last good amounts stay with their date and a warning.

Disputed invoices and former customers stay out of collection work. Changes in access apply again on each invoice page. If a share ends, earlier pages are read again before the list continues.
