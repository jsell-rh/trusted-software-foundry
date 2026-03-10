// Package filter implements a safe, injection-proof filter language for
// querying foundry-postgres resources.
//
// The filter language is a small subset of SQL WHERE-clause expressions
// designed for use as the value of a ?search= query parameter:
//
//	GET /dinosaurs?search=species='Stegosaurus' AND weight>1000
//
// # Grammar
//
//	expr    = term { OR term }
//	term    = factor { AND factor }
//	factor  = [ NOT ] atom
//	atom    = IDENT op value | "(" expr ")"
//	op      = "=" | "!=" | "<" | ">" | "<=" | ">="
//	value   = STRING | NUMBER | BOOLEAN
//
// String literals use single quotes ('foo').
// Boolean literals are the keywords true / false.
// AND / OR / NOT / true / false are case-insensitive keywords.
//
// # Safety guarantees
//
//   - Only fields declared in allowedFields are permitted.
//     Referencing an undeclared field returns ErrUnknownField.
//   - All values are passed as parameterized placeholders ($1, $2, …).
//     No string interpolation into SQL ever occurs.
//   - Operator and field names are validated from a fixed allowlist.
//
// # Example
//
//	whereSQL, args, err := filter.BuildWhere(
//	    "species='Stegosaurus' AND weight>1000",
//	    map[string]bool{"species": true, "weight": true},
//	)
//	// whereSQL: "(species = $1 AND weight > $2)"
//	// args:     []any{"Stegosaurus", float64(1000)}
//
// Zero external dependencies — stdlib only.
package filter

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ErrUnknownField is returned when a filter references a field that is not in
// the allowedFields set.
type ErrUnknownField struct{ Field string }

func (e *ErrUnknownField) Error() string {
	return fmt.Sprintf("filter: unknown field %q", e.Field)
}

// BuildWhere parses search and returns a parameterized SQL fragment and args.
//
// allowedFields is the set of field names the caller permits. If nil or empty,
// no fields are allowed and any field reference returns ErrUnknownField.
//
// The returned whereSQL is a complete, parenthesised expression suitable for
// appending to "WHERE deleted_at IS NULL AND " + whereSQL.
// args are the positional values ($1, $2, …) to pass to db.QueryContext.
//
// Returns ("", nil, nil) when search is empty or whitespace-only.
func BuildWhere(search string, allowedFields map[string]bool) (whereSQL string, args []any, err error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return "", nil, nil
	}

	tokens, err := tokenize(search)
	if err != nil {
		return "", nil, fmt.Errorf("filter: lex: %w", err)
	}

	p := &parser{tokens: tokens, allowed: allowedFields}
	node, err := p.parseExpr()
	if err != nil {
		return "", nil, fmt.Errorf("filter: parse: %w", err)
	}
	if p.pos < len(p.tokens) {
		return "", nil, fmt.Errorf("filter: unexpected token %q after expression", p.tokens[p.pos].val)
	}

	var gen generator
	sql := gen.emit(node)
	return sql, gen.args, nil
}

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

type tokenKind int

const (
	tokIdent  tokenKind = iota // field name or keyword
	tokString                  // 'single-quoted string'
	tokNumber                  // integer or decimal
	tokBool                    // true | false
	tokAnd                     // AND
	tokOr                      // OR
	tokNot                     // NOT
	tokEq                      // =
	tokNeq                     // !=
	tokLt                      // <
	tokGt                      // >
	tokLte                     // <=
	tokGte                     // >=
	tokLParen                  // (
	tokRParen                  // )
)

type token struct {
	kind tokenKind
	val  string // raw text for ident/string/number; empty otherwise
}

func tokenize(s string) ([]token, error) {
	var tokens []token
	i := 0
	for i < len(s) {
		// Skip whitespace.
		if unicode.IsSpace(rune(s[i])) {
			i++
			continue
		}

		// Single-quoted string.
		if s[i] == '\'' {
			j := i + 1
			for j < len(s) && s[j] != '\'' {
				j++
			}
			if j >= len(s) {
				return nil, fmt.Errorf("unterminated string at position %d", i)
			}
			tokens = append(tokens, token{kind: tokString, val: s[i+1 : j]})
			i = j + 1
			continue
		}

		// Operators.
		if i+1 < len(s) && s[i] == '<' && s[i+1] == '=' {
			tokens = append(tokens, token{kind: tokLte})
			i += 2
			continue
		}
		if i+1 < len(s) && s[i] == '>' && s[i+1] == '=' {
			tokens = append(tokens, token{kind: tokGte})
			i += 2
			continue
		}
		if i+1 < len(s) && s[i] == '!' && s[i+1] == '=' {
			tokens = append(tokens, token{kind: tokNeq})
			i += 2
			continue
		}
		switch s[i] {
		case '=':
			tokens = append(tokens, token{kind: tokEq})
			i++
			continue
		case '<':
			tokens = append(tokens, token{kind: tokLt})
			i++
			continue
		case '>':
			tokens = append(tokens, token{kind: tokGt})
			i++
			continue
		case '(':
			tokens = append(tokens, token{kind: tokLParen})
			i++
			continue
		case ')':
			tokens = append(tokens, token{kind: tokRParen})
			i++
			continue
		}

		// Number.
		if s[i] >= '0' && s[i] <= '9' || (s[i] == '-' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9') {
			j := i
			if s[j] == '-' {
				j++
			}
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j++
			}
			tokens = append(tokens, token{kind: tokNumber, val: s[i:j]})
			i = j
			continue
		}

		// Identifier or keyword.
		if isIdentStart(s[i]) {
			j := i
			for j < len(s) && isIdentContinue(s[j]) {
				j++
			}
			word := s[i:j]
			upper := strings.ToUpper(word)
			switch upper {
			case "AND":
				tokens = append(tokens, token{kind: tokAnd})
			case "OR":
				tokens = append(tokens, token{kind: tokOr})
			case "NOT":
				tokens = append(tokens, token{kind: tokNot})
			case "TRUE":
				tokens = append(tokens, token{kind: tokBool, val: "true"})
			case "FALSE":
				tokens = append(tokens, token{kind: tokBool, val: "false"})
			default:
				tokens = append(tokens, token{kind: tokIdent, val: word})
			}
			i = j
			continue
		}

		return nil, fmt.Errorf("unexpected character %q at position %d", s[i], i)
	}
	return tokens, nil
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentContinue(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

// ---------------------------------------------------------------------------
// AST
// ---------------------------------------------------------------------------

type nodeKind int

const (
	nodeAnd nodeKind = iota
	nodeOr
	nodeNot
	nodeCompare // field op value
)

type astNode struct {
	kind  nodeKind
	left  *astNode
	right *astNode

	// for nodeCompare
	field string
	op    string
	value any // string | float64 | bool
}

// ---------------------------------------------------------------------------
// Parser (recursive descent)
// ---------------------------------------------------------------------------

type parser struct {
	tokens  []token
	pos     int
	allowed map[string]bool
}

func (p *parser) peek() (token, bool) {
	if p.pos >= len(p.tokens) {
		return token{}, false
	}
	return p.tokens[p.pos], true
}

func (p *parser) consume() token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

// parseExpr = term { OR term }
func (p *parser) parseExpr() (*astNode, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind != tokOr {
			break
		}
		p.consume()
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = &astNode{kind: nodeOr, left: left, right: right}
	}
	return left, nil
}

// parseTerm = factor { AND factor }
func (p *parser) parseTerm() (*astNode, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind != tokAnd {
			break
		}
		p.consume()
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = &astNode{kind: nodeAnd, left: left, right: right}
	}
	return left, nil
}

// parseFactor = [ NOT ] atom
func (p *parser) parseFactor() (*astNode, error) {
	t, ok := p.peek()
	if ok && t.kind == tokNot {
		p.consume()
		child, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		return &astNode{kind: nodeNot, left: child}, nil
	}
	return p.parseAtom()
}

// parseAtom = IDENT op value | "(" expr ")"
func (p *parser) parseAtom() (*astNode, error) {
	t, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("unexpected end of filter expression")
	}

	if t.kind == tokLParen {
		p.consume()
		node, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		close, ok := p.peek()
		if !ok || close.kind != tokRParen {
			return nil, fmt.Errorf("expected ')'")
		}
		p.consume()
		return node, nil
	}

	if t.kind != tokIdent {
		return nil, fmt.Errorf("expected field name, got %q", t.val)
	}
	p.consume()
	field := t.val

	if !p.allowed[field] {
		return nil, &ErrUnknownField{Field: field}
	}

	opTok, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("expected operator after field %q", field)
	}
	op, err := parseOp(opTok)
	if err != nil {
		return nil, err
	}
	p.consume()

	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	return &astNode{kind: nodeCompare, field: field, op: op, value: val}, nil
}

func parseOp(t token) (string, error) {
	switch t.kind {
	case tokEq:
		return "=", nil
	case tokNeq:
		return "!=", nil
	case tokLt:
		return "<", nil
	case tokGt:
		return ">", nil
	case tokLte:
		return "<=", nil
	case tokGte:
		return ">=", nil
	}
	return "", fmt.Errorf("expected comparison operator, got %q", t.val)
}

func (p *parser) parseValue() (any, error) {
	t, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("expected value")
	}
	p.consume()
	switch t.kind {
	case tokString:
		return t.val, nil
	case tokNumber:
		f, err := strconv.ParseFloat(t.val, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", t.val, err)
		}
		return f, nil
	case tokBool:
		return t.val == "true", nil
	}
	return nil, fmt.Errorf("expected string, number, or boolean, got %q", t.val)
}

// ---------------------------------------------------------------------------
// SQL generator
// ---------------------------------------------------------------------------

type generator struct {
	args []any
}

func (g *generator) nextPlaceholder() string {
	return fmt.Sprintf("$%d", len(g.args))
}

func (g *generator) emit(n *astNode) string {
	switch n.kind {
	case nodeAnd:
		return fmt.Sprintf("(%s AND %s)", g.emit(n.left), g.emit(n.right))
	case nodeOr:
		return fmt.Sprintf("(%s OR %s)", g.emit(n.left), g.emit(n.right))
	case nodeNot:
		return fmt.Sprintf("(NOT %s)", g.emit(n.left))
	case nodeCompare:
		g.args = append(g.args, n.value)
		return fmt.Sprintf("%s %s %s", n.field, n.op, g.nextPlaceholder())
	}
	return ""
}
