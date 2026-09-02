-- Domains written off as parked, when the crawl had only read a forwarding page.
--
-- WHAT WAS WRONG. A site can publish its real address in markup rather than in
-- an HTTP redirect: a document with no content whose whole head is
-- `<meta http-equiv="refresh" content="0; URL=/de">`. The crawler followed HTTP
-- redirects and never this one, so it read a page with no text on it. The
-- triage classifier treats a landing page with no readable text as parked
-- WITHOUT a model call — that is the correct reading of a genuinely empty page
-- and the wrong reading of this one — and the domain settled as `no_site` with
-- no company created. anwr-group.com is the case that surfaced it; every
-- language-gateway site of that vintage has the same shape.
--
-- The crawler now follows such a page once before the classifier sees it, so
-- new triage reads land on the real site. That fix cannot reach a domain
-- already settled: a settled disposition is deliberately final ("stop asking"
-- is what the status means to the next message that arrives), and the sweep
-- only picks up rows whose next_attempt_at is set, which a settled row's is
-- not.
--
-- So the rows the old reading settled are reopened here, and only those.
SET LOCAL lock_timeout = '5s';

UPDATE organization_domain_disposition
SET status = 'pending',
    -- The settled-shape constraint requires source and organization_id to be
    -- absent on a pending row: this domain is genuinely unanswered again, not
    -- answered by the read that got it wrong.
    source = NULL,
    evidence = NULL,
    site_read_id = NULL,
    -- Due now, and the attempt budget returned. The old attempt read a page
    -- the crawler could not follow, so charging the domain for it would let a
    -- site exhaust its budget on a page it never really got to see.
    attempts = 0,
    next_attempt_at = now(),
    updated_at = now()
WHERE status = 'no_site'
  -- Only a verdict a site read produced. A heuristic or a human settled the
  -- domain on other evidence, and neither was misled by the empty page.
  AND source = 'site_read'
  -- The classifier's own sentence for this reading, which is the narrowest
  -- true description of the defect. A read that failed to load, or one that
  -- read real text and found no company, settled correctly and stays settled.
  AND evidence = 'the site classifies as parked: the landing page carries no readable text'
  -- A row that produced a company is not a miss, whatever else it says.
  AND organization_id IS NULL
  -- An admission decision outranks this correction: a domain somebody chose to
  -- suppress must not be quietly put back in the queue.
  AND admission IS DISTINCT FROM 'suppressed';
