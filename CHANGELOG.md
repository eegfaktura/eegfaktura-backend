# Changelog

All notable changes to **eegfaktura-backend (Go REST/GraphQL API)** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and
versioning follows the deployment release tags. Detailed diffs stay in the `git log`;
this changelog highlights the changes relevant for overview and operations.

## [Unreleased]

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
