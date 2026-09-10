-- A name read off a company's public website is its own lead source.
--
-- It weights LOW, like `crawl`: nobody asked us for anything, so the evidence
-- says the person exists and nothing more. Without a row here the source
-- resolves to the neutral default and a crawled name scores like one somebody
-- typed in by hand.
INSERT INTO lead_source (key, label, intent, sort_order, active, system, version)
VALUES ('siteread', 'Website read', 'low', 70, true, true, 1)
ON CONFLICT (key) DO NOTHING;
