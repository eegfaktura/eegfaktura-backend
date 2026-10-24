-- Undo fix.sql: rows the cleanup changed and nobody changed since (modifiedBy still
-- 'cleanup-2026-10-24') get their rejected process state back; last_process_state is cleared.
-- Rows changed later (e.g. a new registration) keep their newer state.
-- The script does not commit. It returns "geaendert" and "tenants": check, then COMMIT or ROLLBACK.
BEGIN;

WITH restored AS (
  UPDATE base.meteringpoint
  SET process_state      = last_process_state,
      last_process_state = NULL,
      "modifiedAt"       = now(),
      "modifiedBy"       = 'rollback-cleanup-2026-10-24'
  WHERE "modifiedBy" = 'cleanup-2026-10-24'
    AND process_state = 'INACTIVE'
    AND last_process_state IN ('INVALID', 'REJECTED')
  RETURNING tenant)
SELECT count(*) AS geaendert, count(DISTINCT tenant) AS tenants FROM restored;

-- COMMIT;   or   ROLLBACK;
