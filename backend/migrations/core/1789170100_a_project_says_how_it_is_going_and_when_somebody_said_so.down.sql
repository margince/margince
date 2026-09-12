SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS project_health_assessment;
DROP FUNCTION IF EXISTS project_health_assessment_is_append_only();
