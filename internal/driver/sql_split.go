package driver

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

func SplitSQLStatements(sqlText, driverName string) []string {
	isMySQL := strings.EqualFold(driverName, "mysql") || strings.EqualFold(driverName, "mariadb")
	return SplitWithDelimiters(sqlText, isMySQL, func(seg string) []string {
		return splitDefaultDelimiter(seg, isMySQL)
	})
}

// dollarTagAt returns the dollar-quote opener at runes[i] ("$$" or "$tag$"), or "" if none.
// A tag cannot start with a digit, so positional parameters like $1 are never mistaken for one.
func dollarTagAt(runes []rune, i int) string {
	if runes[i] != '$' {
		return ""
	}
	// Like PostgreSQL's lexer: "$" inside an identifier (price$usd) never opens a dollar quote.
	if i > 0 && (runes[i-1] == '_' || runes[i-1] == '$' || unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1])) {
		return ""
	}
	j := i + 1
	for j < len(runes) && (runes[j] == '_' || unicode.IsLetter(runes[j]) || (j > i+1 && unicode.IsDigit(runes[j]))) {
		j++
	}
	if j < len(runes) && runes[j] == '$' {
		return string(runes[i : j+1])
	}
	return ""
}

func splitDefaultDelimiter(content string, isMySQL bool) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote, inDoubleQuote, inBacktick := false, false, false
	inPGBackslashString := false // true while inside PostgreSQL E'...' / e'...' escape string
	inLineComment := false
	blockCommentDepth := 0
	dollarTag := "" // non-empty while inside a dollar-quoted body

	runes := []rune(content)
	n := len(runes)

	isQuoteEscaped := func(i int) bool {
		if !isMySQL && !inPGBackslashString {
			return false
		}
		bs := 0
		for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
			bs++
		}
		return bs%2 == 1
	}
	hasPrefixAt := func(i int, s string) bool {
		rs := []rune(s)
		if i+len(rs) > n {
			return false
		}
		for k, r := range rs {
			if runes[i+k] != r {
				return false
			}
		}
		return true
	}

	for i := 0; i < n; i++ {
		r := runes[i]

		if inLineComment {
			current.WriteRune(r)
			if r == '\n' {
				inLineComment = false
			}
			continue
		}
		if blockCommentDepth > 0 {
			current.WriteRune(r)
			if !isMySQL && r == '/' && i+1 < n && runes[i+1] == '*' {
				blockCommentDepth++
				current.WriteRune('*')
				i++
				continue
			}
			if r == '*' && i+1 < n && runes[i+1] == '/' {
				blockCommentDepth--
				current.WriteRune('/')
				i++
			}
			continue
		}
		if dollarTag != "" {
			if hasPrefixAt(i, dollarTag) {
				current.WriteString(dollarTag)
				i += len([]rune(dollarTag)) - 1
				dollarTag = ""
				continue
			}
			current.WriteRune(r)
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && i+1 < n && runes[i+1] == '-' {
				inLineComment = true
				current.WriteString("--")
				i++
				continue
			}
			if isMySQL && r == '#' {
				inLineComment = true
				current.WriteRune('#')
				continue
			}
			if r == '/' && i+1 < n && runes[i+1] == '*' {
				// In MySQL/MariaDB, /*! ... */ and /*M! ... */ are executable SQL: do NOT treat them
				// as opaque comments that hide semicolons.
				if isMySQL && i+2 < n && runes[i+2] == '!' {
					current.WriteString("/*!")
					i += 2
					continue
				}
				if isMySQL && i+3 < n && (runes[i+2] == 'M' || runes[i+2] == 'm') && runes[i+3] == '!' {
					current.WriteString("/*")
					current.WriteRune(runes[i+2])
					current.WriteRune('!')
					i += 3
					continue
				}
				blockCommentDepth = 1
				current.WriteString("/*")
				i++
				continue
			}
			if !isMySQL {
				if tag := dollarTagAt(runes, i); tag != "" {
					dollarTag = tag
					current.WriteString(tag)
					i += len([]rune(tag)) - 1
					continue
				}
				// PostgreSQL E'...' / e'...' escape string literal
				if (r == 'E' || r == 'e') && i+1 < n && runes[i+1] == '\'' && (i == 0 || !(runes[i-1] == '_' || unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1]))) {
					inSingleQuote = true
					inPGBackslashString = true
					current.WriteRune(r)
					current.WriteRune('\'')
					i++
					continue
				}
			}
		}

		switch {
		case r == '\'' && !inDoubleQuote && !inBacktick:
			if inSingleQuote && isQuoteEscaped(i) {
				// Backslash-escaped literal quote (\' in MySQL or PostgreSQL E'...'):
				// stays inside string and must NOT pair with a following closing quote (e.g. E'\'').
				break
			}
			if inSingleQuote && i+1 < n && runes[i+1] == '\'' {
				current.WriteString("''")
				i++
				continue
			}
			inSingleQuote = !inSingleQuote
			if !inSingleQuote {
				inPGBackslashString = false
			}
		case r == '"' && !inSingleQuote && !inBacktick:
			if inDoubleQuote && isQuoteEscaped(i) {
				break
			}
			if inDoubleQuote && i+1 < n && runes[i+1] == '"' {
				current.WriteString(`""`)
				i++
				continue
			}
			inDoubleQuote = !inDoubleQuote
		case r == '`' && !inSingleQuote && !inDoubleQuote:
			if inBacktick && i+1 < n && runes[i+1] == '`' {
				current.WriteString("``")
				i++
				continue
			}
			inBacktick = !inBacktick
		}

		if r == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick {
			if stmt := strings.TrimSpace(current.String()); stmt != "" && HasExecutableSQL(stmt, isMySQL) {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteRune(r)
	}

	if last := strings.TrimSpace(current.String()); last != "" && HasExecutableSQL(last, isMySQL) {
		statements = append(statements, last)
	}
	return statements
}

// SplitWithDelimiters honors MySQL client `DELIMITER xx` lines (used by mysqldump and routine scripts
// so BEGIN ... END bodies keep their inner semicolons). Segments under the default ";" delimiter go
// through split; segments under a custom delimiter are cut at each occurrence of it.
// ponytail: custom-delimiter segments are cut textually (not quote-aware), same as the mysql client's
// common usage; upgrade path: teach the token-aware splitters a variable terminator.
func SplitWithDelimiters(sqlText string, isMySQL bool, split func(string) []string) []string {
	delim := ";"
	var out []string
	var seg strings.Builder
	flush := func() {
		text := seg.String()
		seg.Reset()
		if delim == ";" {
			out = append(out, split(text)...)
			return
		}
		for _, part := range strings.Split(text, delim) {
			if part = strings.TrimSpace(part); part != "" && HasExecutableSQL(part, isMySQL) {
				out = append(out, part)
			}
		}
	}
	for _, line := range strings.SplitAfter(sqlText, "\n") {
		t := strings.TrimSpace(line)
		if len(t) > len("DELIMITER ") && strings.EqualFold(t[:len("DELIMITER ")], "DELIMITER ") {
			flush()
			delim = strings.Fields(t[len("DELIMITER "):])[0]
			continue
		}
		seg.WriteString(line)
	}
	flush()
	return out
}

// errStopScan is an internal sentinel to halt streaming early without reporting an error.
var ErrStopScan = errors.New("stop scan")

// ScanSQLStatements streams SQL statements from r one-by-one into fn using a bounded bufio.Reader
// instead of loading the entire script into memory or converting it to []rune.
// Honors quotes, comments, MySQL /*! / /*M! executable comments, PostgreSQL dollar-quoting and E'...',
// and MySQL DELIMITER directives. Returning ErrStopScan from fn halts scanning cleanly with nil error.
func ScanSQLStatements(r io.Reader, driverName string, fn func(stmt string) error) error {
	isMySQL := !strings.EqualFold(driverName, "postgres") && !strings.EqualFold(driverName, "postgresql") && !strings.EqualFold(driverName, "sqlite") && !strings.EqualFold(driverName, "sqlite3")
	br := bufio.NewReaderSize(r, 64*1024)

	// Strip UTF-8 BOM if present at start of stream.
	if bom, err := br.Peek(3); err == nil && len(bom) == 3 && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
		_, _ = br.Discard(3)
	}

	var current strings.Builder
	delim := ";"
	delimLastRune := ';'

	inSingleQuote, inDoubleQuote, inBacktick := false, false, false
	inPGBackslashString := false
	inLineComment := false
	blockCommentDepth := 0
	dollarTag := ""
	backslashCount := 0
	prevRune := rune(0)
	atLineStart := true

	emitCurrent := func() error {
		stmt := strings.TrimSpace(current.String())
		current.Reset()
		if stmt != "" && HasExecutableSQL(stmt, isMySQL) {
			if err := fn(stmt); err != nil {
				return err
			}
		}
		return nil
	}

	peekString := func(n int) string {
		b, _ := br.Peek(n)
		return string(b)
	}

	readDollarTag := func() string {
		// Current rune '$' was already consumed. Peek up to 64 bytes for optional tag + closing '$'.
		b, _ := br.Peek(64)
		if len(b) == 0 {
			return ""
		}
		// Fast path for $$
		if b[0] == '$' {
			_, _ = br.ReadByte()
			return "$$"
		}
		// Walk runes in peeked slice
		off := 0
		first := true
		for off < len(b) {
			ru, sz := utf8.DecodeRune(b[off:])
			if ru == '$' {
				tag := "$" + string(b[:off+sz])
				_, _ = br.Discard(off + sz)
				return tag
			}
			valid := ru == '_' || unicode.IsLetter(ru) || (!first && unicode.IsDigit(ru))
			if !valid {
				return ""
			}
			first = false
			off += sz
		}
		return ""
	}

	for {
		// Check for DELIMITER directive at start of line when outside quotes/comments/dollar-quotes.
		if atLineStart && !inSingleQuote && !inDoubleQuote && !inBacktick && !inLineComment && blockCommentDepth == 0 && dollarTag == "" {
			// Skip leading horizontal spaces/tabs to detect indented DELIMITER lines without losing them if not DELIMITER.
			peek := peekString(32)
			trimmedLeft := strings.TrimLeft(peek, " \t")
			if len(trimmedLeft) >= len("DELIMITER ") &&
				strings.EqualFold(trimmedLeft[:len("DELIMITER")], "DELIMITER") &&
				(trimmedLeft[len("DELIMITER")] == ' ' || trimmedLeft[len("DELIMITER")] == '\t') {
				line, lineErr := br.ReadString('\n')
				if err := emitCurrent(); err != nil {
					if errors.Is(err, ErrStopScan) {
						return nil
					}
					return err
				}
				fields := strings.Fields(strings.TrimSpace(line)[len("DELIMITER"):])
				if len(fields) > 0 && fields[0] != "" {
					delim = fields[0]
					lastR, _ := utf8.DecodeLastRuneInString(delim)
					delimLastRune = lastR
				}
				backslashCount = 0
				prevRune = '\n'
				atLineStart = true
				if lineErr == io.EOF {
					break
				}
				if lineErr != nil {
					return lineErr
				}
				continue
			}
		}

		r, _, err := br.ReadRune()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if r == '\n' {
			atLineStart = true
		} else if r != '\r' {
			atLineStart = false
		}

		if inLineComment {
			current.WriteRune(r)
			if r == '\n' {
				inLineComment = false
			}
			backslashCount = 0
			prevRune = r
			continue
		}

		if blockCommentDepth > 0 {
			current.WriteRune(r)
			if !isMySQL && r == '/' && strings.HasPrefix(peekString(1), "*") {
				_, _, _ = br.ReadRune()
				current.WriteRune('*')
				blockCommentDepth++
				prevRune = '*'
				backslashCount = 0
				continue
			}
			if r == '*' && strings.HasPrefix(peekString(1), "/") {
				_, _, _ = br.ReadRune()
				current.WriteRune('/')
				blockCommentDepth--
				prevRune = '/'
				backslashCount = 0
				continue
			}
			backslashCount = 0
			prevRune = r
			continue
		}

		if dollarTag != "" {
			if r == '$' && len(dollarTag) >= 2 && strings.HasPrefix(peekString(len(dollarTag)-1), dollarTag[1:]) {
				_, _ = br.Discard(len(dollarTag) - 1)
				current.WriteString(dollarTag)
				dollarTag = ""
				backslashCount = 0
				prevRune = '$'
				continue
			}
			current.WriteRune(r)
			backslashCount = 0
			prevRune = r
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && strings.HasPrefix(peekString(1), "-") {
				_, _, _ = br.ReadRune()
				inLineComment = true
				current.WriteString("--")
				backslashCount = 0
				prevRune = '-'
				continue
			}
			if isMySQL && r == '#' {
				inLineComment = true
				current.WriteRune('#')
				backslashCount = 0
				prevRune = '#'
				continue
			}
			if r == '/' && strings.HasPrefix(peekString(1), "*") {
				_, _, _ = br.ReadRune() // consume '*'
				// MySQL/MariaDB executable comments /*! ... */ and /*M! ... */
				if isMySQL {
					p2 := peekString(2)
					if strings.HasPrefix(p2, "!") {
						_, _, _ = br.ReadRune()
						current.WriteString("/*!")
						backslashCount = 0
						prevRune = '!'
						continue
					}
					if len(p2) >= 2 && (p2[0] == 'M' || p2[0] == 'm') && p2[1] == '!' {
						mRune, _, _ := br.ReadRune()
						_, _, _ = br.ReadRune()
						current.WriteString("/*")
						current.WriteRune(mRune)
						current.WriteRune('!')
						backslashCount = 0
						prevRune = '!'
						continue
					}
				}
				blockCommentDepth = 1
				current.WriteString("/*")
				backslashCount = 0
				prevRune = '*'
				continue
			}
			if !isMySQL {
				if r == '$' {
					if tag := readDollarTag(); tag != "" {
						dollarTag = tag
						current.WriteString(tag)
						backslashCount = 0
						prevRune = '$'
						continue
					}
				}
				if (r == 'E' || r == 'e') && strings.HasPrefix(peekString(1), "'") &&
					(prevRune == 0 || !(prevRune == '_' || unicode.IsLetter(prevRune) || unicode.IsDigit(prevRune))) {
					_, _, _ = br.ReadRune() // consume '\''
					inSingleQuote = true
					inPGBackslashString = true
					current.WriteRune(r)
					current.WriteRune('\'')
					backslashCount = 0
					prevRune = '\''
					continue
				}
			}
		}

		isEscaped := (isMySQL || inPGBackslashString) && (backslashCount%2 == 1)
		switch {
		case r == '\'' && !inDoubleQuote && !inBacktick:
			if inSingleQuote && isEscaped {
				// Backslash-escaped literal quote
			} else if inSingleQuote && strings.HasPrefix(peekString(1), "'") {
				_, _, _ = br.ReadRune()
				current.WriteString("''")
				backslashCount = 0
				prevRune = '\''
				continue
			} else {
				inSingleQuote = !inSingleQuote
				if !inSingleQuote {
					inPGBackslashString = false
				}
			}
		case r == '"' && !inSingleQuote && !inBacktick:
			if inDoubleQuote && isEscaped {
				// Backslash-escaped literal double quote
			} else if inDoubleQuote && strings.HasPrefix(peekString(1), `"`) {
				_, _, _ = br.ReadRune()
				current.WriteString(`""`)
				backslashCount = 0
				prevRune = '"'
				continue
			} else {
				inDoubleQuote = !inDoubleQuote
			}
		case r == '`' && !inSingleQuote && !inDoubleQuote:
			if inBacktick && strings.HasPrefix(peekString(1), "`") {
				_, _, _ = br.ReadRune()
				current.WriteString("``")
				backslashCount = 0
				prevRune = '`'
				continue
			}
			inBacktick = !inBacktick
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if delim == ";" && r == ';' {
				if err := emitCurrent(); err != nil {
					if errors.Is(err, ErrStopScan) {
						return nil
					}
					return err
				}
				backslashCount = 0
				prevRune = r
				continue
			}
		}

		current.WriteRune(r)

		if !inSingleQuote && !inDoubleQuote && !inBacktick && delim != ";" && r == delimLastRune {
			curStr := current.String()
			if strings.HasSuffix(curStr, delim) {
				trimmed := strings.TrimSpace(curStr[:len(curStr)-len(delim)])
				current.Reset()
				if trimmed != "" && HasExecutableSQL(trimmed, isMySQL) {
					if err := fn(trimmed); err != nil {
						if errors.Is(err, ErrStopScan) {
							return nil
						}
						return err
					}
				}
				backslashCount = 0
				prevRune = r
				continue
			}
		}

		if r == '\\' {
			backslashCount++
		} else {
			backslashCount = 0
		}
		prevRune = r
	}

	if err := emitCurrent(); err != nil && !errors.Is(err, ErrStopScan) {
		return err
	}
	return nil
}

// HasExecutableSQL checks whether a SQL statement string contains executable tokens
// (i.e. not entirely comments, whitespace, or empty semicolons).
func HasExecutableSQL(stmt string, isMySQL bool) bool {
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inBlockComment := false
	inLineComment := false

	runes := []rune(stmt)
	n := len(runes)
	tokenCount := 0

	for i := 0; i < n; i++ {
		r := runes[i]
		if inLineComment {
			if r == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if r == '*' && i+1 < n && runes[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && i+1 < n && runes[i+1] == '-' {
				inLineComment = true
				i++
				continue
			}
			if isMySQL && r == '#' {
				inLineComment = true
				continue
			}
			if r == '/' && i+1 < n && runes[i+1] == '*' {
				if isMySQL && i+2 < n && runes[i+2] == '!' {
					// In MySQL, /*! ... */ is a versioned executable comment containing SQL
					tokenCount++
					i += 2
					continue
				}
				if isMySQL && i+3 < n && (runes[i+2] == 'M' || runes[i+2] == 'm') && runes[i+3] == '!' {
					// In MariaDB, /*M! ... */ is an executable comment containing SQL
					tokenCount++
					i += 3
					continue
				}
				inBlockComment = true
				i++
				continue
			}
		}
		if r == '\'' && !inDoubleQuote && !inBacktick {
			inSingleQuote = !inSingleQuote
			tokenCount++
			continue
		}
		if r == '"' && !inSingleQuote && !inBacktick {
			inDoubleQuote = !inDoubleQuote
			tokenCount++
			continue
		}
		if r == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
			tokenCount++
			continue
		}
		if !unicode.IsSpace(r) && r != ';' {
			tokenCount++
		}
	}
	return tokenCount > 0
}

// EscapeSQLString escapes a string literal for the specified SQL driver dialect.
// ponytail: MySQL/MariaDB assumes default backslash-escaping (dump prologues clear NO_BACKSLASH_ESCAPES);
// use EscapeMySQLStringWithMode when NO_BACKSLASH_ESCAPES may be active on a live session.
