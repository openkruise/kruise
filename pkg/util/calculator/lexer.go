package calculator

import (
	"fmt"
	"strings"
	"unicode"
)

// yyLex lexical analyzer
type yyLex struct {
	input string
	pos   int
	line  int
	col   int
	token int
	err   error
}

// init initializes the lexical analyzer
func (x *yyLex) init(input string) {
	x.input = input
	x.pos = 0
	x.line = 1
	x.col = 1
}

// next gets the next character
func (x *yyLex) next() rune {
	if x.pos >= len(x.input) {
		return 0
	}
	r := rune(x.input[x.pos])
	x.pos++
	if r == '\n' {
		x.line++
		x.col = 1
	} else {
		x.col++
	}
	return r
}

// peek looks at the next character without moving position
func (x *yyLex) peek() rune {
	if x.pos >= len(x.input) {
		return 0
	}
	return rune(x.input[x.pos])
}

// skipWhitespace skips whitespace characters
func (x *yyLex) skipWhitespace() {
	for {
		r := x.peek()
		if r == 0 || !unicode.IsSpace(r) {
			break
		}
		x.next()
	}
}

// readNumber reads numbers (including floats, percentages and Kubernetes resource units)
func (x *yyLex) readNumber() string {
	start := x.pos
	seenDot := false
	seenE := false
	seenPercent := false
	expectExponentDigit := false

	for {
		r := x.peek()
		if r == 0 {
			break
		}

		// Handle digits
		if unicode.IsDigit(r) {
			expectExponentDigit = false
			x.next()
			continue
		}

		// Handle decimal point
		if r == '.' {
			if seenDot || seenE || seenPercent {
				break // Can't have multiple dots, or dot after exponent/percent
			}
			seenDot = true
			x.next()
			continue
		}

		// Handle exponent (e or E for scientific notation)
		// Note: Only lowercase 'e' or uppercase 'E' followed by digit/sign is scientific notation
		// Kubernetes units only go up to P/Pi (Petabyte), so 'E' alone is not a valid unit
		if r == 'e' || r == 'E' {
			if seenE || seenPercent {
				break // Can't have multiple exponents, or exponent after percent
			}
			seenE = true
			expectExponentDigit = true
			x.next()
			continue
		}

		// Handle sign in exponent (+ or -)
		if r == '+' || r == '-' {
			// Sign is only allowed immediately after 'e' or 'E'
			if !expectExponentDigit {
				break
			}
			expectExponentDigit = false
			x.next()
			continue
		}

		// Handle percent
		if r == '%' {
			if seenPercent {
				break // Can't have multiple percents
			}
			x.next()
			break // Percent must be the last character of a number
		}

		// Handle Kubernetes resource units (must come after number part)
		if isQuantityUnitChar(r) {
			// If we're in the middle of exponent parsing, stop
			if expectExponentDigit {
				break
			}
			// Read the unit part
			for isQuantityUnitChar(x.peek()) {
				x.next()
			}
			break
		}

		// Any other character stops number reading
		break
	}

	if start >= len(x.input) {
		return ""
	}
	return x.input[start:x.pos]
}

// readIdentifier reads identifiers
func (x *yyLex) readIdentifier() string {
	start := x.pos
	for {
		r := x.peek()
		if r == 0 || !isIdentifierChar(r) {
			break
		}
		x.next()
	}
	if start >= len(x.input) {
		return ""
	}
	return x.input[start:x.pos]
}

// isQuantityUnitChar checks if it's a Kubernetes resource unit character
func isQuantityUnitChar(r rune) bool {
	// Kubernetes resource units: m, k, M, G, T, P, Ki, Mi, Gi, Ti, Pi
	// Note: E/Ei (Exabyte) are not supported - quantities limited to Petabyte
	return unicode.IsLetter(r)
}

// isIdentifierChar checks if it's an identifier character
func isIdentifierChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// isQuantity checks if it's a Kubernetes resource quantity
func (x *yyLex) isQuantity(s string) bool {
	if len(s) == 0 {
		return false
	}

	// Find the end position of the numeric part
	numEnd := 0
	for i, r := range s {
		if !unicode.IsDigit(r) && r != '.' && r != 'e' && r != 'E' && r != '+' && r != '-' {
			numEnd = i
			break
		}
	}

	if numEnd == 0 {
		return false
	}

	unit := s[numEnd:]
	// Valid Kubernetes resource units (limited to Petabyte for practical use)
	// Decimal: m (milli), k (kilo), M (Mega), G (Giga), T (Tera), P (Peta)
	// Binary: Ki (Kibi), Mi (Mebi), Gi (Gibi), Ti (Tebi), Pi (Pebi)
	validUnits := []string{"m", "k", "M", "G", "T", "P", "Ki", "Mi", "Gi", "Ti", "Pi"}

	// Check if unit is in validUnits
	for _, validUnit := range validUnits {
		if unit == validUnit {
			return true
		}
	}
	return false
}

// Lex lexical analyzer interface implementation
func (x *yyLex) Lex(yylval *yySymType) int {
	x.skipWhitespace()

	r := x.next()
	if r == 0 {
		return 0 // EOF
	}

	switch r {
	case '+', '-', '*', '/', '(', ')', ',':
		x.token = int(r)
		return x.token
	case '.':
		// Might be part of a number, rollback and read complete number
		x.pos--
		x.col--
		num := x.readNumber()
		yylval.str = num
		// fmt.Printf("DEBUG: Parsed number from '.': '%s'\n", num)
		if x.isQuantity(num) {
			return QUANTITY
		}
		return NUMBER
	default:
		if unicode.IsDigit(r) {
			// Rollback and read complete number
			x.pos--
			x.col--
			num := x.readNumber()
			yylval.str = num
			// fmt.Printf("DEBUG: Parsed number from digit: '%s'\n", num)
			if x.isQuantity(num) {
				return QUANTITY
			}
			return NUMBER
		} else if unicode.IsLetter(r) {
			// Rollback and read complete identifier
			x.pos--
			x.col--
			ident := x.readIdentifier()
			yylval.str = ident

			// Check if it's a function name or variable
			switch strings.ToLower(ident) {
			case "max", "min":
				return IDENT
			case "cpu", "memory":
				// Variable name, return IDENT for subsequent processing
				return IDENT
			default:
				// Unknown identifier, might be an error
				return IDENT
			}
		} else {
			x.token = int(r)
			return x.token
		}
	}
}

// Error error handling
func (x *yyLex) Error(s string) {
	// Store error in lexer for later retrieval
	x.err = fmt.Errorf("parse error: %s at line %d, col %d", s, x.line, x.col)
}
