# Bulk change — contract fixtures

JSON contract of the routes `/bulkchange/*` between eegfaktura-backend and eegfaktura-admin
(konzept-massenaenderungen.md, Tech Design A). Behind the proxy the routes are `/api/bulkchange/*`.

| File | Route | Content |
|---|---|---|
| `communities.response.json` | `GET /bulkchange/communities` | one entry of the list (`tenant`, `name`, `rcNumber`) |
| `tariffs.response.json` | `GET /bulkchange/tariffs` | active tariffs of the community the rules can set (`id`, `name`, `type` EEG/VZP/EZP, `joiningFee`), sorted by type and name |
| `preview.request.json` | `POST /bulkchange/preview` | body: the rule |
| `preview.response.json` | `POST /bulkchange/preview` | counters, run hints, every hit with class, hints, error |
| `preview.empty.response.json` | `POST /bulkchange/preview` | a rule without hits: counters 0, `rows` an empty list (never `null`) |
| `apply.request.json` | `POST /bulkchange/apply` | body: the rule and the changed hits with expected values |
| `apply.response.json` | `POST /bulkchange/apply` | written hits with hints, skipped hits with reason |
| `preview.file.request.json` | `POST /bulkchange/preview` | rule with action `FROM_FILE` (Teil 2): one value per member number |
| `preview.file.response.json` | `POST /bulkchange/preview` | the file rule's answer: `UNCHANGED_NOT_IN_FILE`, F8, counter `unchangedNotInFile`, list `notFound` |
| `apply.file.request.json` | `POST /bulkchange/apply` | the file rule again (with its values) and the changed hit |
| `apply.file.response.json` | `POST /bulkchange/apply` | the written hit of the file rule |
| `error.response.json` | all | error body (here 409: another run or a tariff archive holds the lock) |

**Origin:** written by hand for the backend PR of the bulk change (feat/bulk-change); the scenario is built in
`api/bulkChangeContract_test.go` (tenant `TE000441`, four members), which compares the live answers of the
handlers with these files. A change of a file is a contract change with eegfaktura-admin (CHANGELOG "Contract").

**Purpose:** eegfaktura-admin keeps copies of these files with the source commit in the file header and develops
and tests its page against them.

Notes for the reader of the contract:

- The tenant goes in the header `tenant`, never in the body. Only the realm role `superuser` passes.
- Codes, not texts, are the contract for hits: classes `CHANGED`, `UNCHANGED_CONDITION`, `UNCHANGED_TARGET`,
  `UNCHANGED_NOT_IN_FILE` (`FROM_FILE` only), `ERROR`; errors `F1`–`F8` (F7/F8 `FROM_FILE` only); hints `H1`–`H10`; run hints `L1`–`L3`; skip reasons `CHANGED_SINCE_PREVIEW` and
  `F1`–`F8`. `oldText`/`newText` are plain German texts of the values (e.g. „Kein SEPA“, tariff name).
- `oldValue`/`newValue` are the stored values (`null` = NULL). admin sends them back unchanged as
  `expectedOld`/`expectedNew`.
- Error codes (body `{"error": {code, error, message}}`): 6100 rule or request invalid (400), 6101 a hit twice
  (400), 6102 a hit not (or no longer) in the community — "preview outdated" (400), 6103 unknown community (404),
  6104 locked (409), 6105 failed, nothing changed (500), 6108 the COMMIT failed, outcome unknown — reload the preview (500), 6106 apply with a token without user name (403), 6107 body over 2 MiB (413; about 13,000 hits of a
  member rule with 35-character values, about 9,600 of the metering point tariff, or 5,000 file values plus 5,000 hits). Missing token 403, invalid token 401, no superuser 403.

- `FROM_FILE` (fields `MANDATE_REFERENCE`, `MANDATE_DATE`): `values` is a list of `{participantNumber, value}`, at
  most 5,000, number and value not empty and at most 100 characters (else 6100). A member is matched by its number,
  trimmed and case-insensitive, otherwise exactly. `counts.unchangedNotInFile` and `notFound` (numbers of the file
  without a member, as written in the file) appear only for `FROM_FILE`, and only when not 0 or empty. Apply sends the rule with its values again.
