# Changelog

All notable changes to **eegfaktura-backend (Go REST/GraphQL API)** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and
versioning follows the deployment release tags. Detailed diffs stay in the `git log`;
this changelog highlights the changes relevant for overview and operations.

## [Unreleased]

### Added
- **Energy data requests: per-meter periods, lock against double requests, live status in Messages**
  (`konzept-smart-nachfordern.md`). Contract change with **eegfaktura-web** (caller of `POST /meteringpoint/syncenergy`
  and `GET /eeg/notifications/{id}`); the request additions are optional, older web versions keep working.
  - `syncenergy`: each entry of `meteringPoints` takes optional `from`/`to` (ms, like the request's own). Set, they
    are the period of that metering point (cut to start/end of day and to its active period as before); missing, the
    request period applies. An optional top-level `complete: [string]` lists metering points web found complete:
    they are not requested and appear in the run/summary as skipped with reason `COMPLETE`; a metering point also
    listed in `meteringPoints` is handled by that entry (requested, or skipped for its own reason). With periods per
    metering point, or without a request period, the run (and so summary and live entry) carries the span of the
    requested metering points as its period.
  - **Lock (1 hour):** a metering point requested by a run of the same tenant in the last hour for a period sharing
    at least one day, and not rejected (`ABLEHNUNG_PT`) there, is not requested again: skipped with reason
    `ALREADY_REQUESTED` and `until` (ms, run creation + 1 h). Single and bulk requests alike; no override. A rejected
    metering point is free at once. GEA tenants are locked separately. A metering point whose request could not be
    sent (MQTT error, answer `500`) is taken out of the run again and is not locked; of a metering point with several
    participations only the span of the sent ones stays.
  - **A request that sends nothing stores no run** (everything skipped: locked, complete, not registered, …): no
    live entry and no summary; the response body reports the skipped metering points. Before, a request for two or
    more metering points that were all skipped got a summary with 0 requested.
  - **Response: `200` with a body instead of `204`** (online community):
    `{"requested": <metering points requested from the grid operator>, "skipped": [{"meteringPoint", "reason",
    "until"?}]}`, the skipped entries the run records (`COMPLETE`, `ALREADY_REQUESTED`, `NOT_REGISTERED`, …), also for
    a single request that is locked. Error status codes are unchanged; an offline community still gets `204`. Web
    (`syncMeteringPoint`) only checks for a 2xx answer and does not read the body, so older web versions keep working.
    A request with only `complete` (every metering point complete) is accepted (nothing stored); before, a request without
    metering points got `500` („no Meter selected“), which it still gets without `complete`.
  - **A run that cannot be stored is not sent:** if the run row cannot be written (database error), nothing goes to
    the grid operator and the request gets `500` (error code `5100`, as a failed send); it can be repeated at once.
    Before, the requests went out without a run, so nothing locked or summarised them. (Rare: if the row is written
    but the answer is lost, the metering points stay locked for the hour although nothing was sent.) The lock and
    the stored requests are only read when at least one metering point can be sent: a request where every metering
    point is complete or skipped for another reason answers `200` even when that read would fail.
  - `skipped[]` entries in the run row and in `ZUSAMMENFASSUNG_PT` gain the optional field `until`.
  - **Notification list (admins):** each open run with a summary (two or more metering points, younger than 24 h,
    not yet summarised) is listed as one entry shaped like a stored `ZUSAMMENFASSUNG_PT` (type `MESSAGE`, process
    `EDA_PROCESS`), computed at request time, with `live: true` in the message, the run's row id as `id` and its
    creation time as `date`; it is ordered by id like the other entries. The `ANTWORT_PT` / `ABLEHNUNG_PT` of such a
    run are left out of the list. Once the stored summary is written it replaces the live entry, and the single
    answers are listed again as before. Single requests are unchanged. The rows behind it are kept for 15 s per
    backend instance (cleared when the instance writes a run or a summary; new answers show after 15 s at the
    latest, so an answer arriving within these 15 s can show once as a single entry before it moves into the live
    entry). If they cannot be read, the list comes without live entries instead of failing. For a tenant without a
    run younger than 24 h only a short probe runs, the rows are not read.
  - **Answers are attributed by period** (summary, lock and live entry alike). Answers carry no period and no
    reference to the request, so per metering point: a later run supersedes an earlier request only for a period
    sharing a day (before: any later run with that metering point, which marked e.g. a May request as „requested
    again“ after an April request and gave it all answers); within its range an answer goes to the oldest request
    without an answer, else to the newest. A rejection frees the period of the request it belongs to.
  - **One row set for summary, lock and live entry:** all three read the run rows and summaries of the last 49 hours
    (2 x deadline + 1 h) and the answers from the oldest of these runs on, of the tenants of the community id, and
    attribute them alike; an answer to an open run keeps its owner in the set even if that run is older than the
    deadline. The 5-minute check reads this window on every run (before: from the oldest open bulk run on, which left
    out single requests after a restart and could disagree with the live entry and the lock). After a long outage
    a request is now still summarised up to about a day after its deadline (before: about an hour).
  - No schema change: the notification JSON is extended additively; old runs have no `until`.
  - **Deploy order:** backend first is safe; web shows the new reasons and the live entry only with its matching
    release.

### Changed
- **Energy data request after an owner change: the old member's part is requested too** (decision of 2026-10-24:
  the billing needs all current data, and the old member's data cannot be got later). **Behaviour towards the grid
  operators changes.** Before, `syncenergy` skipped the old member's moved entry (flag 0, reason `NOT_ASSIGNED`)
  and requested only the new member's part. Now every entry of the metering point is requested for its part of the
  period (cut to its own active period); an archived entry (flag 2) is still skipped (`NOT_ASSIGNED`). Note: when
  an archived metering point is registered again, `saveMeteringPoint` sets its earlier INACTIVE entries to flag 0, so
  their periods are requested too; only archived entries of metering points that never came back stay skipped.
  - Grid operator side (ebUtilities process CR_REQ_PT, https://www.ebutilities.at/prozesse/503, PDF:
    https://www.ebutilities.at/api/export/pdf/processes/503): the grid operator checks „ob der Zählpunkt durch den
    Anforderer im Anforderungszeitraum beliefert wird“ (rejection 55/94). The requester is the community, so the old
    member's period qualifies while the metering point belonged to the community.
  - A merged request goes out with the current entry (flag 1), else the later part's; for BEG its grid operator is
    the receiver.
  - Parts that overlap or touch (old until 15.06., new from 16.06.) go out as **one** ANFORDERUNG_PT over both parts;
    a gap between the owners (old until 02.09., new from 06.09.) gives **two** requests, one per part. No period is
    sent twice. A request only in the old member's period is sent (before: skipped as `NOT_ACTIVE_IN_PERIOD`).
  - The 1 h lock is checked per part: a part requested within the hour is skipped, the other part is sent. The run
    records one period per metering point, the span of the sent parts (as for several participations before), so
    with a gap the days between the owners are locked too (no entry is active there, so none would be sent).
    A part is skipped whole when the lock shares at least one day with it, as for every metering point since the
    lock was introduced (no trimming): when the old member's last day is the new member's first day and only the
    new part was requested, a request over both parts sends nothing for the old part either until the lock ends
    (at most 1 h, the answer says `ALREADY_REQUESTED` with `until`).
  - Skipped reason with several entries: independent of their order, the reason that tells most counts
    (`ALREADY_REQUESTED` before `NOT_REGISTERED`, `NO_ACTIVE_PERIOD`, `NOT_ACTIVE_IN_PERIOD`, `NOT_ASSIGNED`); locked
    for two entries, the later `until`. The old member's entry outside the period gives way to the new member's
    reason, as `NOT_ASSIGNED` did before.
  - Open: whether the grid operators answer a request for the old member's period (whose consent has ended) is not
    verified in production yet (the spec above says it should be accepted). Known limit with a gap (two requests
    for one metering point in a run): the latest answer counts for the summary and the lock, so a rejection of one
    part frees the lock for the whole span and the summary can show the metering point as rejected although the
    other part was accepted; the summary can also be written after the first of the two answers (the second one is
    then listed as a single answer).

### Security
- **`/query` no longer returns the token verifier's error text.** A request to the GraphQL endpoint with an
  invalid or expired token got the verifier message in the 401 body (e.g. `oidc: id token signed with
  unsupported algorithm …`). The body is now empty, as on the JWT-protected REST endpoints; the message stays in the log.
- **Unauthenticated `GET /master/test` removed.** The route had no auth check and only printed „TestApi“ to
  stdout; nothing calls it. `GET /master/test` now answers 404.

### Fixed
- **EDA: ZUSTIMMUNG_ECON/ECOF with code 172 sets the metering point to INVALID again** (platform#180). Code 172
  („Kunde hat Datenfreigabe abgelehnt“) had an empty `case` since 2025-09-22: the line for 173 was inserted between
  the label and the body of 172, and Go has no fallthrough. A consent message with 172 only went to the history; the
  metering point stayed in its state (usually PENDING) and the community got no notification. 172 and 173 now share
  the body again, as introduced on 2024-12-09: process state INVALID, status code 172, a notification for the
  admins. ABLEHNUNG_ECON/ECOF is unchanged. Metering points that received 172 before stay as they are.
- **Metering points with an owner change: each entry gets its own period.** After an owner change a
  metering point has two entries in the community (old member, new member). The lookup behind the energy data
  request and the master data for energystore joined the period only by metering point number, so
  both entries got the new member's period (or every combination of both). Effect: an energy data request over the
  change date sent the request for the new member's part twice to the grid operator. Now every entry carries its
  own period and flag, and the request is sent once. Metering points with one entry return the same as before
  (the old member's part is now requested too, see Changed).
- **Energy data request: an ended entry gets its full last day.** `syncenergy` cut the requested period at the
  entry's `inactivesince` as UTC midnight (e.g. 31.05. 02:00 local), so the last active day was mostly not requested
  from the grid operator; `activesince` started at UTC midnight (01:00 / 02:00 local). `inactivesince` is the last
  active day (energystore and billing count it as a whole day): the period now ends at 23:45 local time of that day
  and starts at 00:00 local time of `activesince`, as the request period itself (summer time included). Affects every
  metering point whose active period starts or ends inside the requested period (single entries too, e.g. a
  metering point that left the community); one with an open end is unchanged — to be confirmed by the user, as it
  changes what goes to the grid operators for all such entries. Pre-existing; with the old member's part now
  requested (see Changed) it reaches the grid operators on every owner change.

### CI
- `rolling-release.yml`: no image build on a draft pull request — it runs when the pull request is marked ready for review (`ready_for_review`) and on every later push to it; `pr-checks.yml` still checks drafts. Pushes, tags and the deploy dispatch are unchanged.

## [1.6.0] – 2026-10-21

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

### Changed
- Mail deliverability: the global default activation mail template declares `lang="de"`
  (was `en`) so clients don't mis-detect the language (minor spam-score signal). Per-tenant
  templates on the data volume are unaffected (operator cleanup). The parser test expects `lang="de"` (it still
  expected `en`; env/billing builds ran no PR checks).
- **Docker image: Go pinned to 1.25.14** (platform#180). Both stages of the `Dockerfile` used the floating tag
  `golang:1.25`; they now use `golang:1.25.14`, the version of the CI.
- **Tests: PostgreSQL 18 in the database tests** (platform#180). The Testcontainers database of the tests used
  `postgres:15-alpine`, while every zone runs PostgreSQL 18; the tests now use `postgres:18.6-alpine`. PostgreSQL 18
  also logs "ready to accept connections" twice (initdb server, then the real server), so the wait for the second line
  stays. No change to the service.
- **Go 1.26: toolchain raised to 1.26.9 and pinned** (platform#180). Both `Dockerfile` stages used the floating
  tag `golang:1.25`; `go.mod` (`go 1.26.0`), CI (`GO_VERSION`) and both stages (`golang:1.26.9`) now use 1.26.9, the newest 1.26 patch release older than
  7 days. It fixes the standard library findings of govulncheck in `net/http`, `net/textproto`, `crypto/tls` and
  `html/template` (GO-2026-6599, -6600, -6603, -6605, -6607, -6608, -6610 to -6613, -6617). Still open:
  GO-2026-6809 (`crypto/x509`), fixed only in Go 1.26.10 (released 2026-10-19, not yet 7 days old).

### Fixed
- **Tests: the middleware tests run without Keycloak** (platform#180). `TestMain` in `api/middleware` called
  `InitKeycloak()` against localhost:7080, so the Protect, ConditionProtect, auth and GQLAdminCheck tests never ran
  (they mock `VerifyTokenClaims`). `TestMain` is removed; `api/middleware` now runs in the blocking unit and full CI
  jobs. The database test helper waits for the second "ready to accept connections" log line of PostgreSQL (the first
  one comes from the temporary initdb server). No change to the service.
- **MQTT broker: data race on the stop channel** (platform#180). `Listen` replaced the stop channel created by
  `NewMessageBroker` inside its own goroutine, while `Stop` reads it from another goroutine. `Listen`
  now uses the channel from the constructor. The broker is started once and stopped at shutdown, as before.
  `mqtt` runs in the blocking full CI job (race detector).
- **Tests: the `api` tests compile again** (platform#180). The metering point and participant controller tests called
  the handler functions from before the `MeteringHandler`/`ParticipantHandler` refactoring and the old
  `QueryParticipant` signature; they now build the handlers with the test database. `api` runs in the blocking full CI
  job; the report-only "Known errors" job is removed, as no package is left in it. No change to the service.
- **CI: `go vet` blocks the unit job** (platform#180). The four findings are fixed without a change in behaviour: keyed
  fields in the struct literals in `api/meteringPointController.go`, `mqtt/subscriptionHandler.go` and
  `eda/edaSubscription.go`, and the unreachable `return` in `util/time.go`.
- **CI: `gofmt` check in the unit job** (platform#180). Six files were not formatted (import order, alignment); they
  are formatted now, and the unit job fails on a tracked `.go` file that `gofmt -l` reports.

### Security
- **Dependencies: `golang.org/x/net` v0.60.0, `golang.org/x/crypto` v0.57.0** (platform#180). Fixes the govulncheck
  findings GO-2026-6603, -6610, -6611, -6612, -6617 (x/net) and GO-2026-6354, -6355 (x/crypto). x/net v0.60.0
  needs x/crypto v0.57.0 and Go 1.26; `go mod tidy` also raises x/mod, x/sync, x/sys, x/text and x/tools (minor).
  Still open: `github.com/golang-jwt/jwt` v3 (GO-2025-3553, no fix in v3) and `github.com/xuri/excelize/v2` v2.11.0
  (GO-2026-6684, -6685, -6688, -6689, fixed only in unreleased commits).

### Notes
- Go 1.26: with `go 1.26.0` the GODEBUG defaults of 1.26 apply (`cryptocustomrand`, `httpservecontentmaxranges`,
  `tlssecpmlkem`, `urlstrictcolons`); none affects the backend. The Green Tea garbage collector is the default:
  watch the pod memory after the first rollout (opt-out at build time: `GOEXPERIMENT=nogreenteagc`). (platform#180)

## [1.5.1] – 2026-10-20

### Changed
- **Master data export: member status from the metering points** (platform#165, decision 20.10.2026). Both „Status“
  columns showed the stored member status, which stays ACTIVE after a member left. They now show „aktiv“ (at least one
  active metering point), „in Anmeldung“ (a metering point not yet active whose registration is running; a metering point
  that left counts as inactive until it is active again) or „inaktiv“ (all left, archived or rejected). A revocation counts once it is confirmed (the metering point's
  status turns INACTIVE then, also with a later end date). Members without metering points have no row in the export,
  as before. Stored data is unchanged; the import does not read the column.

## [1.4.0] – 2026-10-20

### Fixed
- **Excel exports: the filter covers all columns and rows** (customer request 19.10.2026). The master data export set
  its auto filter to `A1:AH10`: the columns AI–AO (Ort, Straße, HausNr., aktiviert, deaktiviert, Zp. Tarifname,
  Umspannwerk) had no filter button, and rows after 10 were outside the range. The metering point list export had the
  same fixed range. Both now span every header column and all rows.

### Added
- **Summary of an energy data request in Messages** (platform#173). A request for two or more metering points
  (dialog „Werte anfordern“) gets one summary notification once every metering point has answered, at the latest
  24 hours after the request: requested, accepted (with the requested period, for the energy data check in web),
  rejected (with the answer texts), no answer, not requested (with the reason: not registered, not assigned, no or no
  active period, not found; these were skipped silently before), and requested again later. The single answer
  notifications stay unchanged. The energy data itself is not tracked: the backend drops CR_MSG since 12.06.2026.
  - No schema change: the request is an additional notification row (process `ENERGY_REQUEST`), hidden from
    Messages; reads are id ranges on the primary key.
  - A check every 5 minutes (and at start) writes the summaries; an advisory lock keeps it to one summary per request.
  - Answers are matched by community id and metering point (GEA tenants sharing a community id included). An answer
    belongs to the request before it; a later request for the same metering point (also a single one, stored without
    summary) takes the answers after it.
  - A request gets no summary if the backend (re)starts more than about an hour after its deadline (the start looks
    back 25 hours).
  - Answers carry no period: an answer that arrives after a later request for the same metering point (also for
    another period) counts for the later one; the earlier summary lists the metering point as requested again.
  - **Deploy order:** web with the matching release first (platform#173), else Messages shows the summary as an
    empty entry.
- CI: PR checks (tests on every pull request) and a security scan (leaked secrets in new commits,
  vulnerable dependencies, Dockerfile/Kubernetes misconfigurations). Dependency and misconfiguration
  findings are only reported and do not block; a leaked secret in new commits fails the check. Jobs for
  known failing tests are report-only.
- CI: every run keeps its scan and test results as artifacts for 30 days (`scan-*`,
  `test-results-<job>`, `known-errors-<job>`) for a CI report collector; the secret scan keeps
  outcomes and counts only.

## [1.3.2] – 2026-10-17

### Added
- GraphQL requests without the admin role are logged (preparation for enforcing the role). Requests
  pass as before; one INFO line `gql-admin-check: non-admin token` per GraphQL operation, with tenant,
  client id (`azp`), `sub`, number of access groups, member flag (`is_user`), superuser flag and
  operation type/name. This log is temporary and is removed when the role is enforced.

### Fixed
- **MQTT command with an invalid online flag** (F2). A `pontononlinestate` command whose `online` field is
  not a boolean is now logged and dropped. Before, it stopped the message broker and with it the service.
- **Negative page size of the process history** (F4). In `GET /process/history` a negative page size (`ps`)
  is now rejected with 400; before, it crashed the request (recovered per request). Paging behaviour is unchanged.
- **Size limit for the master data import** (F5). An uploaded Excel workbook may now unzip to at most
  128 MiB in total (before: the excelize default of 16 GB); larger files are rejected. The shipped test
  workbooks and typical templates unzip to a few MB.
  Temporary files of an import are written to a directory of their own, which is removed afterwards, also
  when the file is rejected.

## [1.3.1] – 2026-10-16

### Fixed
- **Participation factor after an owner change** (support ticket 66110076). After an owner change a
  metering point exists twice in a community: the old member's entry (moved, inactive) and the new
  member's.
  - The confirmation of the grid operator (ANTWORT_CPF) wrote the new factor to both entries, so the old
    member's history got it too. Now only the current entry (flag 1) is updated; the EDA answer names no
    member, so an answer arriving after an owner change goes to the new member.
  - The factor field in the member form failed for such a metering point („No metering factor found
    N:2“), because the lookup matched the entries by number only. It now finds the member's own entry
    and changes only that one.
  - A factor answer that matches no entry is logged instead of being dropped silently.
  - Editing such a metering point (save in the metering point form) answered 400 „More as one or zero
    Meteringpoint are found!“ after the change was already stored, and the mail on deactivation left out
    the direction: the lookup of the current entry also matched the moved entry. It now finds the
    current entry once.

## [1.3.0] – 2026-10-14

### Changed
- **Excel import reports new members without a direct debit type** (platform#143, review F): one
  warning with the number of members the import created with an IBAN but without „Einzugsart“
  (column missing, empty or unknown) — they are not debited until a type is set (web: an empty type
  is no longer collected as CORE, team decision 14.10.).
  Members that already exist are not counted; the import only adds metering points to them.
- Import messages for an unknown or conflicting direct debit type are warnings with their own codes
  (`W_DEBIT_TYPE_UNKNOWN`, `W_DEBIT_TYPE_CONFLICT`) instead of the generic error `E_UNDEFINED_0`.
- **EEG master data export without the column „SEPA“** (platform#143, review H): the EEG switch
  „SEPA aktiv“ is no longer read since web#132 (`base.eeg.sepa` is still written, not read).

### Added
- **Excel master data import reads the direct debit type** (platform#143). Optional column
  „Einzugsart“ (header „DebitType“ works too): `CORE`, `B2B`, `NONE` or „Kein SEPA“; case, blanks
  and hyphens are ignored. Without the column, or with an empty cell, the type stays empty; such a
  member is not debited. An unknown value is reported in the import
  notification and left empty. A member on several rows takes the first non-empty type; a different
  one on a later row is reported. Like every master data column, it only applies to members the
  import creates: for an existing member the import only adds metering points.

## [1.2.0] – 2026-10-11

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
