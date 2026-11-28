-- The page "Prozess History" (fetchEdaHistory: tenant = …, protocol IN (…), date BETWEEN …, ORDER BY date) without a
-- scan of the whole table (platform#282); no data change. CONCURRENTLY: no lock on the table while it is built; the
-- file must hold this single statement (no transaction around it).
CREATE INDEX CONCURRENTLY IF NOT EXISTS processhistory_tenant_protocol_date_idx ON base.processhistory (tenant, protocol, date);
