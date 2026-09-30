# 0015: Zero-Dependency CSV Import with Column Mapping & Excel-Safe Export

To enable fast, reliable data onboarding and spreadsheet interchange without bloating binary size or causing memory spikes, SqlFox implements a native standard-library CSV import engine with visual column mapping and Excel-safe CSV export.

## Problem

1. **Binary Bloat & Memory Overhead**: Third-party spreadsheet engines (such as `excelize/v2`) add ~15MB of binary bloat and consume significant RAM when loading large datasets into intermediate memory structures.
2. **Leading Zero Loss & Type Corruption**: Spreadsheet applications and naive CSV importers routinely strip leading zeros from phone numbers (`08123456789` -> `8123456789`), postal codes (`01234`), and national IDs, or convert them into scientific notation (`8.12E+09`).
3. **Delimiter & Encoding Friction**: International users and regional spreadsheet exports alternate between commas (`,`), semicolons (`;`), and tabs (`\t`), while UTF-8 Byte Order Marks (`\xef\xbb\xbf`) cause column header corruption (`ï»¿id`).
4. **Schema Mismatch**: Uploaded CSV files rarely have exact 1-to-1 column order matching target database tables.

## Solution

1. **Zero-Dependency Streaming Engine**:
   - Built entirely on Go standard library `encoding/csv` and native browser file parsing.
   - Streams rows into parameterized multi-row batch inserts (`INSERT INTO table (...) VALUES (?, ?)...`), keeping peak memory below 20MB even for large files.
   - Backward-compatible `driver.Session.Exec` supporting variadic parameters `args ...any` for safe parameterized execution.

2. **Automatic Delimiter Sniffing & BOM Stripping**:
   - Sniffs line delimiter frequencies (`,`, `;`, `\t`, `|`) across the initial rows to auto-select the right separator, with manual user override.
   - Transparently strips UTF-8 BOM (`\xef\xbb\xbf`) prior to parsing headers.

3. **Visual Column Mapping Matrix**:
   - Native HTML5 `<dialog id="csv-import-dialog">` reads headers and sample values client-side in microseconds.
   - Automatically auto-matches CSV headers to database table columns by case-insensitive name.
   - Supports explicit `[Skip / Do Not Import]` for unneeded columns.

4. **Leading Zero & NULL Preservation**:
   - Values are bound as string parameters to target database columns without intermediary float/integer conversion, keeping `"08123456789"` intact in `VARCHAR`/`TEXT` fields.
   - Configurable "Treat empty values as NULL" option binds empty strings as SQL `NULL`.
   - Fault tolerance options: Rollback on error (strict transaction) vs Skip invalid rows with detailed error reporting.

5. **Excel-Safe CSV Export**:
   - Prepend UTF-8 BOM (`\uFEFF`) to prevent Windows Excel from opening CSVs in Windows-1252/ANSI.
   - Values with leading zeros (`0812...`) or sensitive symbols are formatted as `="0812..."` formulas so Excel treats them as exact strings.

## Consequences

- Zero new external dependencies; maintains single-binary lightness (<25MB).
- Developers can reliably import real-world client data (phone numbers, SKU codes, addresses) without data corruption.
- Full compatibility with Excel, LibreOffice, and Google Sheets.
