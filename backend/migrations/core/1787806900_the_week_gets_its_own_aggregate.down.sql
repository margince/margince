-- Children first: weekly_review_deal references the review it belongs to.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS weekly_review_deal;
DROP TABLE IF EXISTS weekly_review;
