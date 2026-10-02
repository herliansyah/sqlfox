# Issue 04: Global Web UI Error Boundary & Sidebar Content Visibility

Status: resolved
Type: task
Feature: large-scale-hardening

## Goal
Implement native browser CSS `content-visibility: auto` on sidebar table elements, 150ms debounce on schema filter, and a global network error boundary preserving editor tabs and staged modifications.

## Requirements
- Add `content-visibility: auto; contain-intrinsic-size: 0 28px;` styling to schema tree items.
- Debounce `filterSchemaTree` by 150ms.
- Add global network reconnection banner with exponential backoff while retaining editor tab content and staged edits.

## Answer
Implemented in `web/dist/index.html`:
- Added `style="content-visibility: auto; contain-intrinsic-size: 0 28px;"` to table and view items in `renderSchemaTree` for browser-native layout skipping of off-screen nodes.
- Added 150ms debouncer (`schemaFilterDebounceTimer`) to `filterSchemaTree` and immediate execution on `clearSchemaFilter`.
- Implemented `#global-reconnect-banner` with exponential backoff (1s, 2s, 4s, 8s) in `apiFetch`, cleanly intercepting network failures, preserving editor tabs, queries, and staged grid edits without data loss.

