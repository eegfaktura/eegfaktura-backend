# Changelog

Alle nennenswerten Änderungen an **eegfaktura-backend (Go REST/GraphQL-API)** werden hier dokumentiert.

Das Format orientiert sich an [Keep a Changelog](https://keepachangelog.com/de/1.1.0/),
die Versionierung an den Deployment-Release-Tags. Detail-Diffs bleiben im `git log`;
dieser Changelog hebt die für Überblick und Betrieb relevanten Änderungen hervor.

## [Unreleased]

## [1.0.0] – 2026-06-28

Erster vollständig aus öffentlichem Quellcode gebauter Produktiv-Release
(einheitlicher Source-Build-Cutover der eegfaktura-Suite).

### Fixed
- Benachrichtigungen: `notification.date` wird in UTC gespeichert statt in lokaler
  Server-Wanduhrzeit; behebt eine Verschiebung um den TZ-Offset in der Anzeige. (#6)
- Auth: Autorisierung über `access_groups` (`/EEG_ADMIN`, `/EEG_USER`) statt über
  Realm-Rollen. (#5)

### Changed
- CI: self-building Dockerfile aus frischem Clone (Stage-1 Source-Build); Push in den
  Development-Tier der Registry mit Auto-Rollout-Bridge (dispatch-deploy). (#2, #3)
- README mit Service-Überblick und Tech-Stack ergänzt. (#4)
