SET LOCAL lock_timeout = '5s';

DELETE FROM retention_policy WHERE object_type = 'deal_risk_day';

DROP TABLE IF EXISTS deal_risk_verdict;
DROP TABLE IF EXISTS deal_risk_day;
