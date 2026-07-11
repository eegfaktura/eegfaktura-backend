# Changelog

All notable changes to **eegfaktura-backend (Go REST/GraphQL API)** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and
versioning follows the deployment release tags. Detailed diffs stay in the `git log`;
this changelog highlights the changes relevant for overview and operations.

## [Unreleased]

### Added
- **Time-of-use tariffs (ZVT)**: `base.tariff` gains `useTimeTariff` plus two
  optional named time windows (`timeTariff{1,2}Active/Name/From/To/CentPerKWh`;
  `time` columns, 15-min raster, `From > To` crosses midnight). `centPerKWh`
  stays the base price; existing tariffs are unchanged (`useTimeTariff=false`).
  Both views are extended: `base.activeTariff` passes the new fields through
  (From/To as `HH:MM` text) and `base.billing_masterdata` exposes them as
  `tariff_use_time_tariff` / `tariff_time{1,2}_*` for the billing service.
  Saving a tariff validates the windows server-side: only consumer/producer
  tariffs, active windows need from/to/price on the 15-min raster,
  `Von ≠ Bis`, the two windows must not overlap (cyclic check, midnight
  crossing allowed) and free kWh are rejected in time-based mode.
  Migration `20260711120000_zvt_time_tariff` (up/down; `schema.sql`,
  `schema.hcl` and `atlas.sum` updated in sync).

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
