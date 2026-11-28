-- The answer to a revoke (FindHistoryMessage: tenant = …, "conversationId" = …, type = …, latest by date) without a
-- scan of the whole table (platform#282); no data change. CONCURRENTLY: no lock on the table while it is built; the
-- file must hold this single statement (no transaction around it).
CREATE INDEX CONCURRENTLY IF NOT EXISTS processhistory_tenant_conversation_idx ON base.processhistory (tenant, "conversationId");
