-- Ended metering points with a rejected re-registration go back to process state INACTIVE
-- (konzept-zaehlpunkt-ausblenden.md, part 2). The rejected state is kept in last_process_state,
-- statusCode stays. Run check.sql first and note the total row ("anzahl").
-- Re-running changes nothing (last_process_state IS NULL). rollback.sql undoes it.
--
-- The script does not commit. It returns "geaendert" (rows changed) and "tenants":
--   both equal to the total row of check.sql  -> COMMIT;
--   otherwise                                -> ROLLBACK;  and check again.
-- (A row can drop out between check and fix if an EDA answer changes it; then run check.sql again.)
BEGIN;

WITH changed AS (
  UPDATE base.meteringpoint
  SET last_process_state = process_state,
      process_state      = 'INACTIVE',
      "modifiedAt"       = now(),
      "modifiedBy"       = 'cleanup-2026-10-24'
  WHERE flag = 1
    AND status = 'INACTIVE'
    AND inactivesince IS NOT NULL
    AND process_state IN ('INVALID', 'REJECTED')
    AND last_process_state IS NULL
  RETURNING tenant)
SELECT count(*) AS geaendert, count(DISTINCT tenant) AS tenants FROM changed;

-- COMMIT;   or   ROLLBACK;
