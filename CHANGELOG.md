# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Visual Table Structure Designer**: HeidiSQL-inspired schema designer with native dialogs, indexed column configurators, and cross-database autocomplete.
- **Visual Query Plan Tree & Grid**: Graphical explain plan inspector with bottleneck badges, cost breakdowns, and offline schema introspection tooling.
- **Pure Go PostgreSQL Native Driver**: High-performance PostgreSQL connector with native catalog introspection without external cgo dependencies.
- **Spreadsheet CRUD & Inline Editing**: Data grid inline cell editing with staged apply mode, revert capabilities, and transaction safety.
- **Real-Time Processlist Manager**: Live running query diagnostics, process killer, redundant index detector, table diagnostics, privilege matrix, and session audit logs.
- **Inline Table Filter Bar**: Quick filter bar with server-side pushdown and seamless handoff to full SQL queries.
- **Safe DDL Guardrails**: Pre-flight lock contention inspection, blocking query detectors, and migration safety checks.
- **Cluster & Replication Inspector**: Dynamic cluster topology monitor with replication lag telemetry and bottleneck identification.
- **Performance Flight Recorder**: On-demand flight recorder capturing correlated process snapshots and metric histories.
- **Interactive Visual ERD Canvas**: Draggable interactive entity-relationship diagram with foreign key connectors and schema exports.
- **FK-Aware Mock Data Generator**: Smart test data generation respecting referential integrity and foreign key closures.
- **Heuristic Index Advisor**: Automated index recommendations, index bloat guards, and safe DDL migration generation.
- **Headless CLI Suite**: Automated query execution runner, cross-connection data transfer, and declarative cron task runner.
- **Streaming SQL Dumps & Row Sync**: Fast streaming schema/data export, referential slice seeding, and cross-connection synchronization.
- **In-App Self-Update Engine**: Native zero-dependency self-update mechanism with platform-specific binary replacement and graceful restart.
- **Author Attribution & Support**: Integrated creator credits and Saweria sponsorship link in status bar footer.

### Changed
- Streamlined router and middleware using Go standard libraries, pruning Chi and WebSocket dependencies.
- Unified environment status badge as single source of truth across UI surfaces.
- Hardened memory ceilings and high-concurrency benchmarks for low-footprint operation (<100MB RAM).

### Fixed
- Handled Windows executable locking during in-app self-update via rename staging.
- Synchronized update restart hooks to eliminate timing races on Windows CI runners.
- Fixed Windows path resolution in declarative job specification JSON loading.

---

## [v0.1.0] - 2026-10-05

### Added
- Initial release of SqlFox, the lightweight, zero-dependency database manager.
- Zero-cgo embedded database drivers for MySQL/MariaDB and SQLite.
- Multi-IP network binding with sequential linear port hunting on address collision.
- Remote Access Guard requiring Master PIN authentication for non-loopback network connections.
- Localized CodeMirror SQL editor with fast contextual schema autocomplete.
- In-memory Connection Vault with AES-GCM credential persistence and portable encrypted archives.
- AI SQL Assistant supporting chat sessions, dynamic schema column injection, and automatic error healing.
- Multi-OS cross-compilation pipeline supporting Linux, macOS, and Windows (amd64 and arm64).
