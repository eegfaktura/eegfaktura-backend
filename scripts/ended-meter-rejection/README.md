# Ended metering points with a rejected re-registration: cleanup (2026-10-24)

Since this release the backend keeps a metering point whose participation has ended (current entry `flag = 1`,
status `INACTIVE`, `inactivesince` set) at process state `INACTIVE` when the grid operator rejects a new
registration; the rejected state goes to `last_process_state`, the code stays in `statusCode`. Before, such entries
got `INVALID` or `REJECTED` and web showed them in every period among the problem cases.

These scripts bring the existing entries in line. The operator runs them once in **prod and pilot, after the
deploy** (so no new cases come in with the old image).

1. **Check** (`check.sql`, read only): one row per tenant and a total row (`tenant` empty) with `anzahl`,
   `tenants`, `invalid`, `rejected`, `ohne_datum` and up to five examples (metering point, process state,
   statusCode, inactivesince). `ohne_datum` counts the same rows without `inactivesince`: they are reported only;
   neither `fix.sql` nor the backend rule changes them (no end date, so the participation period is unclear).
   Clarify them one by one with the community.
2. **Correct** (`fix.sql`): sets `last_process_state` to the current process state, the process state to
   `INACTIVE`, `"modifiedAt" = now()`, `"modifiedBy" = 'cleanup-2026-10-24'`. It does not commit and returns
   `geaendert` (rows) and `tenants`. Both equal to the total row of `check.sql` → `COMMIT`; otherwise `ROLLBACK`
   and check again (an EDA answer may have changed a row in between). Running it again changes nothing.
3. **Check again**: `check.sql` returns only the total row with `anzahl` 0.

**Undo** (`rollback.sql`): entries still marked `"modifiedBy" = 'cleanup-2026-10-24'` get their process state back
from `last_process_state` (`last_process_state` is cleared). Entries changed since keep their newer state. Also
without commit: check the count, then `COMMIT` or `ROLLBACK`.

Nothing else changes: status, `activesince`/`inactivesince`, `statusCode`, the notifications and the billing data
stay as they are. No message goes to the grid operator, no mail is sent.
