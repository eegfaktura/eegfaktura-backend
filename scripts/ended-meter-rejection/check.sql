-- Ended metering points with a rejected re-registration (konzept-zaehlpunkt-ausblenden.md, part 2).
-- Read only. One row per tenant plus a total row (tenant NULL); fix.sql changes exactly these rows.
-- beispiele: up to five metering points per tenant with process state, statusCode and inactivesince.
SELECT m.tenant,
       count(*) AS anzahl,
       count(*) FILTER (WHERE m.process_state = 'INVALID') AS invalid,
       count(*) FILTER (WHERE m.process_state = 'REJECTED') AS rejected,
       (array_agg(m.metering_point_id || ' ' || m.process_state || ' ' || coalesce(m."statusCode"::text, '-')
                  || ' ' || m.inactivesince ORDER BY m.metering_point_id))[1:5] AS beispiele
FROM base.meteringpoint m
WHERE m.flag = 1
  AND m.status = 'INACTIVE'
  AND m.inactivesince IS NOT NULL
  AND m.process_state IN ('INVALID', 'REJECTED')
  AND m.last_process_state IS NULL
GROUP BY GROUPING SETS ((m.tenant), ())
ORDER BY m.tenant IS NULL DESC, anzahl DESC, m.tenant;
