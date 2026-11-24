package safety

import (
	"strings"
)

type tokenKind int

const (
	tokWord tokenKind = iota
	tokIdent
	tokString
	tokLParen
	tokRParen
	tokDot
	tokComma
	tokSemi
	tokOther
)

type sqlToken struct {
	kind     tokenKind
	raw      string
	upper    string
	unquoted string
	start    int
	end      int
}

// tokenizeSQL performs dialect-aware tokenization across MySQL, PostgreSQL, and SQLite.
// Comments (--, #, and nested /* ... */) are skipped so inline comments like DELETE/*x*/FROM
// cannot disguise keywords, while string literals ('...', $$...$$, $tag$...$tag$) and quoted
// identifiers (`...`, "...", [...]) are preserved as atomic tokens with exact byte offsets.
// When mysqlEscapes is true, backslash escapes inside single-quoted strings and # comments are enabled;
// when false, standard SQL (PostgreSQL/SQLite) rules apply.
func tokenizeSQL(q string, mysqlEscapes bool) []sqlToken {
	n := len(q)
	tokens := make([]sqlToken, 0, n/6+4)

	for i := 0; i < n; {
		c := q[i]

		// Whitespace
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' {
			i++
			continue
		}

		// Single-line comments (-- or # in MySQL mode)
		if (c == '-' && i+1 < n && q[i+1] == '-') || (mysqlEscapes && c == '#') {
			for i < n && q[i] != '\n' {
				i++
			}
			continue
		}

		// Block comments /* ... */ (supports PostgreSQL nested block comments when !mysqlEscapes)
		if c == '/' && i+1 < n && q[i+1] == '*' {
			depth := 1
			j := i + 2
			for j < n && depth > 0 {
				if q[j] == '*' && j+1 < n && q[j+1] == '/' {
					depth--
					j += 2
					continue
				}
				if !mysqlEscapes && q[j] == '/' && j+1 < n && q[j+1] == '*' {
					depth++
					j += 2
					continue
				}
				j++
			}
			i = j
			continue
		}

		// PostgreSQL dollar-quoted string literal: $$...$$ or $tag$...$tag$
		if !mysqlEscapes && c == '$' {
			j := i + 1
			for j < n && isIdentByte(q[j]) {
				j++
			}
			if j < n && q[j] == '$' && (j == i+1 || !(q[i+1] >= '0' && q[i+1] <= '9')) {
				tag := q[i : j+1]
				endIdx := strings.Index(q[j+1:], tag)
				stop := n
				if endIdx >= 0 {
					stop = j + 1 + endIdx + len(tag)
				}
				tokens = append(tokens, sqlToken{
					kind:  tokString,
					raw:   q[i:stop],
					start: i,
					end:   stop,
				})
				i = stop
				continue
			}
		}

		// Single-quoted string literal ('...')
		if c == '\'' {
			j := i + 1
			for j < n {
				if mysqlEscapes && q[j] == '\\' && j+1 < n {
					j += 2
					continue
				}
				if q[j] == '\'' {
					if j+1 < n && q[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			tokens = append(tokens, sqlToken{
				kind:  tokString,
				raw:   q[i:j],
				start: i,
				end:   j,
			})
			i = j
			continue
		}

		// Quoted identifiers: `...`, "...", [...]
		if c == '`' || c == '"' || c == '[' {
			closeChar := c
			if c == '[' {
				closeChar = ']'
			}
			j := i + 1
			for j < n {
				if q[j] == closeChar {
					if closeChar != ']' && j+1 < n && q[j+1] == closeChar {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			raw := q[i:j]
			unq := raw
			if len(raw) >= 2 && raw[len(raw)-1] == closeChar {
				unq = raw[1 : len(raw)-1]
			}
			tokens = append(tokens, sqlToken{
				kind:     tokIdent,
				raw:      raw,
				upper:    strings.ToUpper(unq),
				unquoted: unq,
				start:    i,
				end:      j,
			})
			i = j
			continue
		}

		// Punctuation
		switch c {
		case '(':
			tokens = append(tokens, sqlToken{kind: tokLParen, raw: "(", start: i, end: i + 1})
			i++
			continue
		case ')':
			tokens = append(tokens, sqlToken{kind: tokRParen, raw: ")", start: i, end: i + 1})
			i++
			continue
		case '.':
			tokens = append(tokens, sqlToken{kind: tokDot, raw: ".", start: i, end: i + 1})
			i++
			continue
		case ',':
			tokens = append(tokens, sqlToken{kind: tokComma, raw: ",", start: i, end: i + 1})
			i++
			continue
		case ';':
			tokens = append(tokens, sqlToken{kind: tokSemi, raw: ";", start: i, end: i + 1})
			i++
			continue
		}

		// Bare word / identifier / keyword
		if isIdentByte(c) || c == '$' || c == '@' {
			j := i + 1
			for j < n && (isIdentByte(q[j]) || q[j] == '$') {
				j++
			}
			raw := q[i:j]
			tokens = append(tokens, sqlToken{
				kind:     tokWord,
				raw:      raw,
				upper:    strings.ToUpper(raw),
				unquoted: raw,
				start:    i,
				end:      j,
			})
			i = j
			continue
		}

		tokens = append(tokens, sqlToken{kind: tokOther, raw: q[i : i+1], start: i, end: i + 1})
		i++
	}

	return tokens
}

// parseQualifiedTarget parses `[schema.]table` starting at idx, returning (db, table, rawExpr, nextIdx).
func parseQualifiedTarget(tokens []sqlToken, idx int) (string, string, string, int) {
	if idx >= len(tokens) {
		return "", "", "", idx
	}
	first := tokens[idx]
	if first.kind != tokWord && first.kind != tokIdent {
		return "", "", "", idx
	}
	if idx+2 < len(tokens) && tokens[idx+1].kind == tokDot {
		second := tokens[idx+2]
		if second.kind == tokWord || second.kind == tokIdent {
			// Support 3-part catalog.schema.table if present
			if idx+4 < len(tokens) && tokens[idx+3].kind == tokDot {
				third := tokens[idx+4]
				if third.kind == tokWord || third.kind == tokIdent {
					return second.unquoted, third.unquoted, first.raw + "." + second.raw + "." + third.raw, idx + 5
				}
			}
			return first.unquoted, second.unquoted, first.raw + "." + second.raw, idx + 3
		}
	}
	return "", first.unquoted, first.raw, idx + 1
}

// findTopLevelKeyword returns the token index and byte offset of the first matching keyword
// at parenthesis depth 0 starting from startIdx.
func findTopLevelKeyword(tokens []sqlToken, startIdx int, keyword string) (int, int, bool) {
	depth := 0
	for i := startIdx; i < len(tokens); i++ {
		switch tokens[i].kind {
		case tokLParen:
			depth++
		case tokRParen:
			if depth > 0 {
				depth--
			}
		case tokWord:
			if depth == 0 && tokens[i].upper == keyword {
				return i, tokens[i].start, true
			}
		}
	}
	return -1, -1, false
}
