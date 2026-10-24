-- Ended metering points with a rejected re-registration (konzept-zaehlpunkt-ausblenden.md, part 2).
-- Read only. One row per tenant plus a total row (tenant NULL).
--   anzahl     rows fix.sql changes (status INACTIVE, inactivesince set, INVALID/REJECTED); the total row
--              is the "expected" value for fix.sql
--   tenants    tenants with such rows
--   ohne_datum the same without inactivesince: reported only, fix.sql and the backend rule leave them alone
--   beispiele  up to five rows per tenant (metering point, process state, statusCode, inactivesince)
SELECT m.tenant,
       count(*) FILTER (WHERE m.inactivesince IS NOT NULL) AS anzahl,
       count(DISTINCT m.tenant) FILTER (WHERE m.inactivesince IS NOT NULL) AS tenants,
       count(*) FILTER (WHERE m.inactivesince IS NOT NULL AND m.process_state = 'INVALID') AS invalid,
       count(*) FILTER (WHERE m.inactivesince IS NOT NULL AND m.process_state = 'REJECTED') AS rejected,
       count(*) FILTER (WHERE m.inactivesince IS NULL) AS ohne_datum,
       (array_agg(m.metering_point_id || ' ' || m.process_state || ' ' || coalesce(m."statusCode"::text, '-')
                  || ' ' || m.inactivesince ORDER BY m.metering_point_id)
          FILTER (WHERE m.inactivesince IS NOT NULL))[1:5] AS beispiele
FROM base.meteringpoint m
WHERE m.flag = 1
  AND m.status = 'INACTIVE'
  AND m.process_state IN ('INVALID', 'REJECTED')
  AND m.last_process_state IS NULL
GROUP BY GROUPING SETS ((m.tenant), ())
ORDER BY m.tenant IS NULL DESC, anzahl DESC, m.tenant;
