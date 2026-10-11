# Changelog

All notable changes to **eegfaktura-backend (Go REST/GraphQL API)** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and
versioning follows the deployment release tags. Detailed diffs stay in the `git log`;
this changelog highlights the changes relevant for overview and operations.

## [Unreleased]

### Added
- **Mail to the member when a metering point is no longer part of the community**
  (platform#116, concept `konzept-zaehlpunkt-inaktiv-mail.md`). Subject "Dein Zählpunkt ist nicht
  mehr Teil der Energiegemeinschaft"; it names the metering point, its direction, the end date
  (consent end) and who ended the data release. The community gets a copy (Cc), same sending path
  as the activation mail. Triggers:
  - `AUFHEBUNG_CCMI` (grid operator) and `AUFHEBUNG_CCMC` (member): only when the metering point
    really went from status ACTIVE to INACTIVE; a redelivery, a metering point that was not active
    yet, or a revocation that could not be applied sends no mail.
  - EEG deregistration: only with the grid operator's confirmation (`ANTWORT_CCMS` with code 176),
    not with the community's own request (`AUFHEBUNG_CCMS`) and not with a rejection.
  - No mail for manual changes (archiving, editing in the UI or admin).

  One mail per metering point and member address (a GEA with several tenants per RC number sends it
  once). Parallel MQTT handlers cannot send it twice: the revoked rows are locked (`FOR UPDATE`)
  before their previous status is read, and the commit error is checked before any mail goes out.
  A failed mail never undoes the revocation; the community gets an error notification. Without a
  consent end the mail names no date. New
  global template `zp-inactive-mail-template` (embedded, can be overridden per tenant like the other
  templates).

### Fixed
- **Revocations of the data release were dropped when a metering point exists in several
  communities.** `AUFHEBUNG_CCMI`/`AUFHEBUNG_CCMC` carry no community id; the lookup also matched
  rows without consent id in other communities and gave up with "Meteringpoint … is not unique"
  (13× in Prod within 30 days). The metering point stayed active, and neither history nor
  notification was written. The lookup now tries the receiving community (MQTT topic) and the exact
  consent id first. The receiving community is resolved via its RC number, so a GEA with several
  tenants per RC number (`GC100019-001`, `-002`, …) is revoked in all of them; a hit in several
  tenants is only accepted when they share one RC number. The migration placeholder consent id
  `Migration` counts as "no consent id". A row of another community without consent id is only
  used when the receiving community is unknown, so a revocation can no longer end another
  community's participation. If it still fails, the message is kept in the history of the
  receiving community (all its tenants, without notification) instead of being lost.
- `ANTWORT_CCMS` without code 176 is now written to the history as well (without notification).
- eda tests compile again (`FindMeteringByStatus` without context).

### Changed
- **Grid operator of a metering point is derived from its number** (platform#107). On create,
  participant registration, Excel import, update and when the metering point number changes, the
  backend sets `grid_operator_id` to the first 8 characters of the metering point number, for EEG
  and BEG alike. Values sent by the client are never taken over: a full update derives from the
  number in the path, and a partial update of `gridOperatorId`/`gridOperatorName` is refused (400).
  Any other partial update fills in the grid operator when none is stored yet; this includes
  system updates such as `activesince`/`inactivesince` from EDA answers, and a failing lookup there
  does not stop the update. A metering point number whose first 8 characters are not `AT` + 6
  digits is not derived: an existing value is kept, a new metering point gets none (the web
  dialog no longer accepts such numbers). The name comes from `base.gridoperators` (lowest name
  per id; fallback: the EEG's own grid operator name).
  **Effect on the EDA receiver:** for **BEG**, `getReceiverFrom` takes the metering point's stored
  value, so every BEG metering point written after the release is sent to the derived operator.
  For **EEG** the receiver stays the EEG's grid operator, except for the participation factor
  change (CPF), where the web sends the stored metering point value.
  **Deploy order:** this backend before eegfaktura-web with platform#107 (the web no longer
  prefills the grid operator; with an old backend a new BEG metering point would get none).
- New config `grid-operator-alias` translates grid operator numbers that are only reachable under
  another number (Energienetze Steiermark `AT008200` … → `AT008000`). Read once at start; invalid,
  chained or circular entries are logged as ERROR and ignored, an empty list as WARN. Sub-operators
  that are reachable themselves (e.g. Netz OÖ `AT003470`) are not on the list.
- Excel import: the column "Netzbetreiber" is optional and only compared. A different value or an
  alias translation is reported as `W_GRID_OPERATOR_IGNORED` in the import log; rows are no longer
  skipped because column A is empty (`E_PARTICIPANT_1002` is gone). The header row is also found
  via the "Zählpunkt" column.
- `scripts/nb-alias-107/`: check and correction scripts for existing metering points (run by the
  operator).
- `config.yaml`: default `eda-process-versions` raised to the schema sets valid since 2026-10-05
  (ANFORDERUNG_ECON 02.40, ECOF 02.30, ECP 02.10, CPF 01.10). The grid operators deactivated the
  old sets, so the old defaults were rejected by the Ponton messenger. Needs eda-xp >= 1.0.7.

## [1.1.4] – 2026-10-05

### Security
- The `PARTICIPANT_TENANT_ENFORCE` switch is gone: access to a participant of another
  community is now always refused. The switch (introduced in 1.1.1 for a log-only rollout
  phase) could turn the tenant check into logging only; every environment runs with the check
  on. An environment that still sets `PARTICIPANT_TENANT_ENFORCE=false` is no longer affected
  by it.

## [1.1.3] – 2026-10-05

### Security
- The admin gRPC participant update (`UpdateParticipantValues`) applies the same
  rules as the REST partial update: the participant must belong to the given
  tenant, and every key must name an updatable field; all keys are checked
  before the first write.
- Adding a new version of a tariff (`POST /eeg/tariff` with an existing `id`)
  is only possible for a tariff of the caller's own tenant, and deactivating
  the previous version is scoped to the tenant as well.

### Fixed
- `Test_RegisterMeteringPoint` expects the participant tenant query added in
  #59.

## [1.1.2] – 2026-10-05

### Security
- Partial-update endpoints (metering point, participant) now resolve the
  client-supplied field name against the target model and reject names that are
  unknown or not updatable, instead of passing them to the SQL builder verbatim.
  New helper `model.AllowedUpdateColumn` / `model.IsAllowedParticipantUpdatePath`
  with unit tests.
- GraphQL `updateEegModel` and `masterDataUpload` now take the tenant from the
  verified request context (as the `eeg` query already does) and ignore the
  tenant passed as an argument.
- The EEG update (`POST /eeg`, GraphQL and the admin gRPC call) now maps every
  field to a known, updatable column of the EEG and rejects anything else; until
  now an unknown key was passed to the SQL builder as a column name verbatim. New
  helper `model.ResolveFlatUpdateColumn`, which also covers the embedded address,
  account, contact and website fields and accepts column names such as
  `creditor_id` that the web sends.
- The generic EEG update drops write-protected fields (`tenant`, `rcNumber`,
  `communityId`, `online`, `createdAt`) instead of writing them; the web sends
  single fields, other callers may round-trip the whole object. (#58)
- The full participant update (`PUT /participant/{id}`) checks the tenant before
  any write, including the child tables (addresses, contact, bank data). (#57)
- Moving a metering point and registering one on a participant check the tenant
  of the target participant as well. (#59)
- The `/master` API (basic auth) requires the `EEG_ADMIN` group, like the
  token-based endpoints. (#59)

### Fixed
- After the Ponton registration admin-backend could no longer switch a community
  online: the generic EEG update drops `online` since the write-protection change,
  so the admin gRPC call now sets it through the dedicated online-state update.

## [1.1.1] – 2026-10-04

### Security
- **Five single-participant operations ignored the tenant.** `GET`/`PUT`/`DELETE` on a
  participant, the partial update and the confirm step all resolved the row by `id` alone, so
  an authenticated user who knew a participant ID of a *different* community could read,
  change or delete that record — including bank details, contact data and addresses. The
  middleware did validate the `tenant` header against the token, but the verified value was
  only ever written to the log, never applied to the query. IDs offer no protection either:
  they come from `uuid.NewUUID()`, which is time-based rather than random.
  All five now go through `assertParticipantTenant` first. Reported by an external
  contributor who reviewed the code and reported privately rather than opening an issue.
  A staged rollout is possible: `PARTICIPANT_TENANT_ENFORCE=false` logs cross-tenant access
  without rejecting it, so an environment can be observed before the check is switched on.
  The full `PUT /participant/{id}` and the metering-point updates were already scoped
  correctly and served as the template.
- `google.golang.org/grpc` 1.81.0 -> 1.83.1, closing CVE-2026-84304 (HIGH): heap memory
  exhaustion through HTTP/2 DATA frame fragmentation. 1.82.1 — the version Dependabot
  originally proposed — only closes the earlier GHSA-hrxh-6v49-42gf, which is why the bump
  went straight to 1.83.1. The gRPC server is cluster-internal rather than exposed at the
  ingress, which limits who can reach it, but does not remove the exposure. (#41)
- `google.golang.org/grpc` 1.83.1 -> 1.83.2 (Dependabot #50), patch release on top of the
  CVE-2026-84304 fix above.

### Fixed
- Two database connections were leaked on every `archiveTariff` call: both lookup queries
  discarded their `*sql.Rows` without closing them, and Go sets no finalizer on `Rows`, so
  those connections never returned to the pool. `getGridOperators` leaked the same way on its
  scan-error path and ignored `rows.Err()`. Both are admin-triggered and rare, so they do not
  by themselves explain the production pool exhaustions of 2026-07-19 and 2026-08-11 — see #45.
- A participant created through the API without a `residentAddress` block no longer produces an
  address row with an empty `type`. Every read path joins `base.address` on
  `type = 'RESIDENCE'` / `'BILLING'`, so such a row is invisible: the member list of the *whole
  tenant* fails with `converting NULL to string is unsupported`, and the update path matches no
  rows, so the address cannot be repaired through the UI either. Seen in production on
  2026-09-08 and 2026-09-11 in two tenants. Rows already broken need a separate data repair.

### Added
- CI builds `env/**` branches and deploys the resulting image into the matching feature
  environment (ADR-0008): a push to `env/<name>` pins this service in namespace `env-<name>`
  to that branch's `sha-…` image. Previously only the default branch, tags and `preview/**`
  produced an image at all. The environment itself is still provisioned manually.
- The connection pool counters (`open`, `inUse`, `idle`, `maxOpen`, `waitCount`,
  `waitDuration`) are now logged once a minute, and at `WARN` once `inUse` reaches 80% of
  `maxOpen`. This is diagnostic groundwork for #45: it distinguishes a leaking pool
  (`inUse` climbing monotonically and never falling back) from a merely saturated one
  (`waitCount` growing while `inUse` fluctuates) — a question two production incidents left
  open. Interval configurable via `database.statsLogInterval`; a negative value disables it.

## [1.1.0] – 2026-09-07

### Security
- `github.com/xuri/excelize/v2` 2.10.1 -> 2.11.0, closing CVE-2026-54063 (CVSS 7.5). The
  library parses the master-data spreadsheets users upload, so the vulnerable code is on a
  path reachable with attacker-supplied input — unlike the x/crypto advisories, this one is
  worth taking seriously. Resolution also moved `x/crypto` 0.52.0 -> 0.53.0 and `x/net`
  0.54.0 -> 0.56.0 plus the usual indirects.

### Changed
- Excel master-data import: the "Gemeinschafts-ID" column (marked required in the template)
  is now actually enforced. Every data row must carry the community id of the EEG the file
  is uploaded into (case-insensitive); rows with a different id — the classic "wrong file /
  wrong community selected" mistake — and rows with an **empty** cell are rejected and
  reported in the import notification (one message per distinct wrong id). Previously the
  column was ignored entirely, so a file for another community imported without any warning.
  **Note for existing files:** templates that left the column empty must fill it in once.
  Test fixture alignment: the test-DB EEG `TE100200` now carries the same community id as
  the fixture workbook and the env-billing seed (`AT00999900000TC100200000000000002` —
  the SQL fixture was the lone outlier).

### Changed
- Excel master-data import hardening (follow-up to the field fixes):
  - "Zählpunktstatus" now tolerates case and surrounding spaces (` active ` no longer
    rejects the row).
  - An unknown "Energierichtung" value (e.g. a typo like `GENERATON`) now rejects the
    row with an import notification instead of silently importing the metering point as
    CONSUMPTION — a producer imported as consumer corrupts billing. An empty value keeps
    the documented CONSUMPTION default.
  - Duplicate "MitgliedsNr" values are reported as warnings in the import notification:
    numbers used by several members within the file, and numbers already assigned to a
    *different* existing member (re-import rows carrying the member's own number stay
    silent). Rows are still imported — the warning is informational.
- New cleaned import template `tests/260716-vorlage-import-stammdaten.xlsx`: the nine
  columns the importer never reads (Ortsgebiet, Stiege/Stock/Tür/Adresszusatz,
  Überschusseinspeisung, Energiequelle, Verteilungsmodell, Meter Codes) are removed so
  admins no longer fill fields that silently go nowhere. Import stays header-driven, so
  older templates keep working; a regression test pins the shipped template to the
  importer.

### Fixed
- Excel master-data import: three fields from the current import template
  ("250310-vorlage-import-stammdaten") were imported wrongly or not at all:
  - **"Mitglied seit"** is now stored as the member's `participantSince`. Previously the
    importer read a "Dokument unterschrieben" column that does not exist in the template,
    and `saveParticipant` unconditionally overwrote the value with the import date — every
    imported member appeared to have joined "today". The overwrite now only applies as a
    default when no date is provided (also honors a caller-supplied date on registration).
  - **"registriert seit"** (metering point registered-since) now accepts real Excel date
    cells. Rows are read in raw mode, so date-formatted cells arrive as Excel serial
    numbers (e.g. `45292`), which the previous `d.m.yyyy`-only parser rejected — the value
    silently fell back to Jan 1 of the current year. Serial and text dates are now both
    parsed (same for the mandate date). The metering point's `registeredSince` also comes
    from this column now instead of "Mitglied seit" (crossed wiring with the fix above).
  - **"Zugeteilte Menge in Prozent"** now feeds the participation factor (`partFact`).
    The importer only knew the legacy "Teilnehmerfaktor"/"PartFact" headers, so the
    template column never matched and every metering point got 100 %. Plain numbers,
    `%`-suffixed values, decimal commas and percent-formatted cells (raw fraction, e.g.
    `0.5` = 50 %) are handled; the legacy headers still work as fallback.
- Excel master-data import robustness:
  - A failing member no longer aborts the whole import. Each member runs in its own
    transaction; errors are collected and reported in the import notification while the
    remaining rows are still imported (previously everything after the first failure —
    e.g. a duplicate active metering point — was silently dropped).
  - Silently skipped rows now surface in the import notification: rows that look like
    data but have a missing/invalid "Netzbetreiber" (column A), and rows whose name
    cannot be derived ("Name 1" empty and "Name 2" not splittable). A trailing space in
    the "Netzbetreiber" value no longer discards the row (value is trimmed).
- Test suite: the `database` package tests had drifted uncompilable (missing
  `context.Context` arguments after the DAO signature change, stale `createdAt`
  expectation) and are fixed to compile and pass again; new regression tests cover the
  import fixes (incl. an end-to-end continue-on-error/re-import test on its own tenant).
- EDA: `eda-process-versions.AUFHEBUNG_CCMS` bumped `01.10` → `01.30` in the committed
  (local-dev) `config.yaml`. This string is stamped onto the outbound `MessageCodeVersion`
  (`mqtt/messageBroker.go`) and eda-xp uses it to pick the CMRevoke XSD + `schemaLocation`
  (`CMRevokeRequest.getVersion`): `01.10` builds the superseded `cmrevoke/01p00` schema,
  `01.30` the current `cmrevoke/01p10` (`CM_REV_SP/01.30`). Prod already ran `01.30`; the
  repo default and dev overlays had drifted behind — aligned so new environments don't
  emit revocations under an outdated EDA process version.

## [1.0.7] – 2026-07-05

### Added
- The EEG entity now exposes its creation date (`base.eeg.createdat`) via the API as
  `createdAt` (ISO `YYYY-MM-DD`). The column already existed; it is now mapped read-only
  (`skipinsert`/`skipupdate`, DB default `now()` stays authoritative). The web billing
  period selector uses it as the lower bound for EEGs without energy data, so quarterly
  billing runs (e.g. the platform-fee EEG `RC000000`) stay selectable after the quarter
  they belong to has passed.

## [1.0.6] – 2026-07-05

### Fixed
- Mail delivery no longer fails on recipient addresses with leading/trailing whitespace
  (a prod log review found 73 failed sends across 11 tenants in one week, most of them
  addresses like `' mail@x.at'`): both send paths (`SendMail` and — previously completely
  unvalidated — `SendMailWithAttachment`, the ZP list mail) now normalize to/cc per
  `;`-separated part (unicode trim incl. NBSP) and send the **normalized** value, validated
  against a shared address rule (`model.ValidateEmailList`: ASCII local part, TLD ≥ 2 letters,
  no TLD allowlist). A failed ZP list mail now raises an `N_TYPE_ERROR` admin notification
  instead of being log-only.

### Added
- Server-side e-mail enforcement on every write path (the web form alone was the only guard):
  participant create/update/partial-update (`contact.email`), the EEG master data e-mail
  (recipient of the ZP list mail) and the Excel master-data import all normalize and validate
  the address before persisting. Invalid addresses are rejected (API) or imported without
  e-mail plus a visible import-log entry (Excel); an address that is empty after trimming is
  stored as NULL so the send-path guard (`Contact.Email.Valid`) stays meaningful.
- `mail.proto`: additive `SendMailReply.rejectedRecipients` field — the mail server (eda-xp)
  can report recipients it refused; both senders surface them as an error so callers raise the
  existing admin notification instead of losing recipients silently. Backward compatible (old
  eda-xp simply never sets the field); Go stubs regenerated.

### Changed
- CI: Preview-Deployments (ADR-0007) — Push auf `preview/**` baut+deployt on-demand in die Dev-Zone (sha-pinned, kein `:latest`), Auto-Reset bei Branch-Delete.
- Mail templates are now embedded in the binary (`public/templates`) as defaults and resolved
  through an `fs.FS`: at runtime a per-tenant templates dir on the data volume still overrides
  them first, then the global dir; only when neither holds the requested file are the embedded
  defaults used. A fresh deployment therefore renders the activation and ZP-completion mails
  (template, config and inline logo) without any template being hand-seeded onto the PVC, while
  operators keep full per-tenant/global override control on the volume. `ParseTemplate` and
  `ReadActivationMailTemplateConfig` now take an `fs.FS`; the stale unused `parser/templates`
  embed (old logo) was dropped in favour of the single `public/templates` source.

### Fixed
- `TestReadActivationMailTemplateConfig` asserted the wrong inline picture name (`Logo_Faktura.png`);
  the global activation template references `eegfaktura-logo.png`.


## [1.0.5] – 2026-07-04

### Fixed
- ZP completion ("Zählpunkt aktiv") mail: removed a redundant `<br>` before "Mit besten Grüßen".
  Combined with the paragraph's own margin it produced two blank lines in a row; the normal
  single paragraph gap remains.
- ZP completion ("Zählpunkt aktiv") mail never rendered: the `zp-complete-mail-template`
  references `{{.MeteringPoint}}`, but the template data only exposed `Meteringpoints []string`
  → `can't evaluate field MeteringPoint` → "Error Sending Mail" on every completion. Add a
  `MeteringPoint` field to the template data so the mail renders. (#19)
- Mail template resolution now falls back to the global templates dir when a tenant is missing
  the *specific* template file (previously only when the whole tenant template dir was missing),
  fixing "Config file is missing" for the completion mail on tenants that only have the
  activation template. (#19)
- ZP completion mail: `{{.Eeg.ContactPerson}}` rendered the raw `null.String` struct
  (`{{value true}}`); use `.String` with a `Valid` guard like the phone line. (#19)

### Changed
- ZP completion mail template now matches the activation mail: informal "du" wording,
  identical signature/footer (description, address, phone/email/website, "versandt durch"),
  and the logo capped at `max-height: 90px`. (#19)
- ZP completion mail gets its own subject "Dein Zählpunkt ist aktiv" instead of reusing
  "Aktivierung im Serviceportal"; `meteringPointPerformAnswerMsg` now takes the subject as a
  parameter (activation mail keeps its subject). (#19)
- Tests: `trimString` now also strips `\r` so golden template comparisons are CRLF-insensitive;
  `TestGetTemplateFor` builds its expected path with `filepath.Join` (OS-independent);
  `TestManualSending` is skipped unless `RUN_MANUAL_MAIL_TESTS` is set (needs a live mail service). (#19)

## [1.0.4] – 2026-07-01

### Fixed
- Admin master update: the `INACTIVESINCE` update never took effect because the
  parsed inactive-since timestamp was scanned into the `activeSince` variable, so
  `inactiveSince` stayed invalid and the handler returned 501. Scan it into
  `inactiveSince` (also fixes the process-state → INACTIVE path). (#17)

## [1.0.3] – 2026-06-30

### Fixed
- Register goqu's postgres dialect so prepared queries bind `$1` placeholders instead of `?` (fixes EEG loading failing with `pq: syntax error`). (#14)
- SQL injection: bind the `json_to_recordset` input in `MeteringPointChangePartFactor` instead of string-interpolating it. (#15)
- Security: `getEegById`/`getEegByEcId` now build their queries with goqu
  prepared statements (bind parameters) instead of interpolated SQL, removing
  the Snyk Code SQL-injection findings on `database/eegDao.go`. (Snyk `go/Sqli`)

## [1.0.2] – 2026-06-29

### Fixed
- EDA Consent Management (`CM_REV_SP`): a rejection (`ABLEHNUNG_CCMS`) arrives
  without a `<meter>` element, which dereferenced a nil pointer and crashed the
  whole backend; the MQTT broker then crash-looped (QoS-1 redelivery) for every
  tenant. The metering point and reason codes are now read from `responseData`,
  the rejection is recorded as a notification, and the data release is kept
  active (the metering point is no longer revoked on a rejection). Additionally,
  any panic inside an MQTT protocol handler is now recovered so a single message
  can never take down the process. (#10)

## [1.0.1] – 2026-06-28

### Fixed
- Notifications: `notification.date` is stored in UTC instead of the server's local
  wall-clock time; fixes a TZ-offset shift in the displayed time. (#6)

## [1.0.0] – 2026-06-28

First production release built entirely from public source (unified
source-build cutover of the eegfaktura suite).

### Fixed
- Auth: authorize via `access_groups` (`/EEG_ADMIN`, `/EEG_USER`) instead of realm
  roles. (#5)

### Changed
- CI: self-building Dockerfile from a fresh clone (stage-1 source build); push to the
  registry's development tier with an auto-rollout bridge (dispatch-deploy). (#2, #3)
- Added README with service overview and tech stack. (#4)
