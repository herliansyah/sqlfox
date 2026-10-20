# 0056: Table Designer Focus Retention and Relational Constraint Builders

SqlFox enhances the Table Designer and Create Table modal with in-place DOM updates to eliminate focus loss during column editing, adds an interactive floating ENUM/SET Value Builder with syntax validation, dialect-aware cascading Foreign Key selectors with type guards, and composite Index builders with staged modification lifecycles.

## Considered Options

- **Full-Table DOM Re-rendering on Input**: Destroying and re-creating `tbody.innerHTML` on every keystroke (`oninput`) caused continuous caret jumps and loss of input focus during column attribute edits.
- **In-Place DOM Mutation and Dedicated Floating Sub-Editors**: Keeping active input elements intact in the DOM while updating staged state in memory, combined with an anchored floating tag popover for complex `ENUM`/`SET` definitions, guarantees zero cursor displacement and fluid keyboard interaction.
- **Free-Text Column Inputs for Indexes and Foreign Keys**: Allowed typos and schema mismatches that failed only upon remote database execution (e.g. MySQL errno 150).
- **Cascading Schema-Aware Pickers with Type Compatibility Guards**: Dynamically fetching referenced table columns from the active catalog and verifying datatype parity before DDL compilation eliminates constraint errors.

## Consequences

- Continuous column editing in Table Designer and Create Table modal retains 100% cursor and caret stability.
- ENUM and SET columns are cleanly managed through a keyboard-friendly tag popover with automatic single-quote formatting, empty-option prevention, and duplicate detection.
- Foreign Key and Index definitions are validated pre-flight against the active database schema catalog.
- Engine dialect differences (MySQL vs SQLite) are enforced at the UI boundary, preventing invalid DDL generation.
