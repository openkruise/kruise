%{
package calculator

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// parseNumber parses numbers, supports percentages
func (c *Calculator) parseNumber(s string) (float64, error) {
	if strings.HasSuffix(s, "%") {
		numStr := strings.TrimSuffix(s, "%")
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, err
		}
		return num / 100, nil
	}
	return strconv.ParseFloat(s, 64)
}

// parseQuantity parses a Kubernetes quantity string
func (c *Calculator) parseQuantity(s string) (resource.Quantity, error) {
	return resource.ParseQuantity(s)
}

// toQuantity converts a Value to a Quantity
func (c *Calculator) toQuantity(v *Value) *resource.Quantity {
	if v.IsQuantity {
		q := v.Quantity.DeepCopy()
		return &q
	}
	// Convert number to Quantity
	return resource.NewQuantity(int64(v.Number), resource.DecimalSI)
}

// add performs addition operation
func (c *Calculator) add(left, right *Value) *Value {
	if left == nil || right == nil {
		return nil
	}
	if left.IsQuantity || right.IsQuantity {
		// At least one is Quantity, convert both to Quantity for operation
		leftQ := c.toQuantity(left)
		rightQ := c.toQuantity(right)
		result := leftQ.DeepCopy()
		result.Add(*rightQ)
		return &Value{IsQuantity: true, Quantity: result}
	}
	// Both are numbers
	return &Value{IsQuantity: false, Number: left.Number + right.Number}
}

// sub performs subtraction operation
func (c *Calculator) sub(left, right *Value) *Value {
	if left == nil || right == nil {
		return nil
	}
	if left.IsQuantity || right.IsQuantity {
		// At least one is Quantity, convert both to Quantity for operation
		leftQ := c.toQuantity(left)
		rightQ := c.toQuantity(right)
		result := leftQ.DeepCopy()
		negRightQ := rightQ.DeepCopy()
		negRightQ.Neg()
		result.Add(negRightQ)
		return &Value{IsQuantity: true, Quantity: result}
	}
	// Both are numbers
	return &Value{IsQuantity: false, Number: left.Number - right.Number}
}

// mul performs multiplication operation
func (c *Calculator) mul(left, right *Value) *Value {
	if left == nil || right == nil {
		return nil
	}
	if left.IsQuantity && right.IsQuantity {
		// Both are Quantity, invalid
		c.lastError = fmt.Errorf("multiplication of two quantities")
		return nil
	}
	if left.IsQuantity {
		// left is Quantity, right is number
		result := left.Quantity.DeepCopy()
		result.SetMilli(int64(float64(left.Quantity.MilliValue()) * right.Number))
		return &Value{IsQuantity: true, Quantity: result}
	}
	if right.IsQuantity {
		// right is Quantity, left is number
		result := right.Quantity.DeepCopy()
		result.SetMilli(int64(float64(right.Quantity.MilliValue()) * left.Number))
		return &Value{IsQuantity: true, Quantity: result}
	}
	// Both are numbers
	return &Value{IsQuantity: false, Number: left.Number * right.Number}
}

// div performs division operation
func (c *Calculator) div(left, right *Value) *Value {
	if left == nil || right == nil {
		return nil
	}
	if right.IsQuantity {
		// right is Quantity, invalid
		c.lastError = fmt.Errorf("division by quantity")
		return nil
	}
	if left.IsQuantity {
		// left is Quantity, right is number
		if right.Number == 0 {
			c.lastError = fmt.Errorf("division by zero")
			return nil
		}
		result := left.Quantity.DeepCopy()
		result.SetMilli(int64(float64(left.Quantity.MilliValue()) / right.Number))
		return &Value{IsQuantity: true, Quantity: result}
	}
	// Both are numbers
	if right.Number == 0 {
		c.lastError = fmt.Errorf("division by zero")
		return nil
	}
	return &Value{IsQuantity: false, Number: left.Number / right.Number}
}

// callFunc calls a function
func (c *Calculator) callFunc(name string, args []*Value) *Value {
	if len(args) == 0 {
		c.lastError = fmt.Errorf("function %s requires arguments", name)
		return nil
	}
	switch strings.ToLower(name) {
	case "max":
		return c.maxFunc(args)
	case "min":
		return c.minFunc(args)
	default:
		c.lastError = fmt.Errorf("unknown function: %s", name)
		return nil
	}
}

// maxFunc implements max function
func (c *Calculator) maxFunc(args []*Value) *Value {
	if len(args) != 2 {
		c.lastError = fmt.Errorf("max function requires exactly 2 arguments, got %d", len(args))
		return nil
	}
	
	left, right := args[0], args[1]
	if left == nil || right == nil {
		return nil
	}
	
	if left.IsQuantity && right.IsQuantity {
		// Both are Quantity
		cmp := left.Quantity.Cmp(right.Quantity)
		if cmp >= 0 {
			return &Value{IsQuantity: true, Quantity: left.Quantity}
		}
		return &Value{IsQuantity: true, Quantity: right.Quantity}
	}
	
	if left.IsQuantity {
		// Left is Quantity, right is number
		leftQ := left.Quantity.DeepCopy()
		rightQ := resource.NewQuantity(int64(right.Number), resource.DecimalSI)
		cmp := leftQ.Cmp(*rightQ)
		if cmp >= 0 {
			return &Value{IsQuantity: true, Quantity: left.Quantity}
		}
		return &Value{IsQuantity: true, Quantity: *rightQ}
	}
	
	if right.IsQuantity {
		// Right is Quantity, left is number
		leftQ := resource.NewQuantity(int64(left.Number), resource.DecimalSI)
		rightQ := right.Quantity.DeepCopy()
		cmp := leftQ.Cmp(rightQ)
		if cmp >= 0 {
			return &Value{IsQuantity: true, Quantity: *leftQ}
		}
		return &Value{IsQuantity: true, Quantity: rightQ}
	}
	
	// Both are numbers
	if left.Number >= right.Number {
		return &Value{IsQuantity: false, Number: left.Number}
	}
	return &Value{IsQuantity: false, Number: right.Number}
}

// minFunc implements min function
func (c *Calculator) minFunc(args []*Value) *Value {
	if len(args) != 2 {
		c.lastError = fmt.Errorf("min function requires exactly 2 arguments, got %d", len(args))
		return nil
	}
	
	left, right := args[0], args[1]
	if left == nil || right == nil {
		return nil
	}
	
	if left.IsQuantity && right.IsQuantity {
		// Both are Quantity
		cmp := left.Quantity.Cmp(right.Quantity)
		if cmp <= 0 {
			return &Value{IsQuantity: true, Quantity: left.Quantity}
		}
		return &Value{IsQuantity: true, Quantity: right.Quantity}
	}
	
	if left.IsQuantity {
		// Left is Quantity, right is number
		rightQ := resource.NewQuantity(int64(right.Number), resource.DecimalSI)
		cmp := left.Quantity.Cmp(*rightQ)
		if cmp <= 0 {
			return &Value{IsQuantity: true, Quantity: left.Quantity}
		}
		return &Value{IsQuantity: true, Quantity: *rightQ}
	}
	
	if right.IsQuantity {
		// Right is Quantity, left is number
		leftQ := resource.NewQuantity(int64(left.Number), resource.DecimalSI)
		cmp := leftQ.Cmp(right.Quantity)
		if cmp <= 0 {
			return &Value{IsQuantity: true, Quantity: *leftQ}
		}
		return &Value{IsQuantity: true, Quantity: right.Quantity}
	}
	
	// Both are numbers
	if left.Number <= right.Number {
		return &Value{IsQuantity: false, Number: left.Number}
	}
	return &Value{IsQuantity: false, Number: right.Number}
}

// getVariable gets a variable value
func (c *Calculator) getVariable(name string) (*Value, bool) {
	val, exists := c.variables[strings.ToLower(name)]
	if !exists {
		c.lastError = fmt.Errorf("undefined variable: %s", name)
	}
	return val, exists
}

%}

%union {
	val  *Value
	str  string
	num  float64
}

%token <str> NUMBER QUANTITY IDENT
%type <val> expr term factor func_call
%left '+' '-'
%left '*' '/'
%right UMINUS

%%

input:
	expr {
		yylex.(*yyLex).calc.result = $1
	}
	;

expr:
	expr '+' expr {
		calc := yylex.(*yyLex).calc
		$$ = calc.add($1, $3)
		if $$ == nil && calc.lastError == nil {
			calc.lastError = fmt.Errorf("invalid addition operation")
		}
	}
	| expr '-' expr {
		calc := yylex.(*yyLex).calc
		$$ = calc.sub($1, $3)
		if $$ == nil && calc.lastError == nil {
			calc.lastError = fmt.Errorf("invalid subtraction operation")
		}
	}
	| term {
		$$ = $1
	}
	;

term:
	term '*' term {
		calc := yylex.(*yyLex).calc
		$$ = calc.mul($1, $3)
		if $$ == nil && calc.lastError == nil {
			calc.lastError = fmt.Errorf("invalid multiplication operation")
		}
	}
	| term '/' term {
		calc := yylex.(*yyLex).calc
		$$ = calc.div($1, $3)
		if $$ == nil && calc.lastError == nil {
			calc.lastError = fmt.Errorf("invalid division operation")
		}
	}
	| factor {
		$$ = $1
	}
	;

factor:
	NUMBER {
		calc := yylex.(*yyLex).calc
		num, _ := calc.parseNumber($1)
		$$ = &Value{IsQuantity: false, Number: num}
	}
	| QUANTITY {
		calc := yylex.(*yyLex).calc
		q, err := calc.parseQuantity($1)
		if err != nil {
			calc.lastError = err
			$$ = nil
		} else {
			$$ = &Value{IsQuantity: true, Quantity: q}
		}
	}
	| IDENT {
		// Handle variable reference
		calc := yylex.(*yyLex).calc
		varName := strings.ToLower($1)
		if val, exists := calc.getVariable(varName); exists {
			$$ = val
		} else {
			$$ = nil
		}
	}
	| '(' expr ')' {
		$$ = $2
	}
	| func_call {
		$$ = $1
	}
	| '-' factor %prec UMINUS {
		if $2 == nil {
			$$ = nil
		} else if $2.IsQuantity {
			negQ := $2.Quantity.DeepCopy()
			negQ.Neg()
			$$ = &Value{IsQuantity: true, Quantity: negQ}
		} else {
			$$ = &Value{IsQuantity: false, Number: -$2.Number}
		}
	}
	;

func_call:
	IDENT '(' expr ')' {
		calc := yylex.(*yyLex).calc
		$$ = calc.callFunc($1, []*Value{$3})
		if $$ == nil && calc.lastError == nil {
			calc.lastError = fmt.Errorf("function call failed")
		}
	}
	| IDENT '(' expr ',' expr ')' {
		calc := yylex.(*yyLex).calc
		$$ = calc.callFunc($1, []*Value{$3, $5})
		if $$ == nil && calc.lastError == nil {
			calc.lastError = fmt.Errorf("function call failed")
		}
	}
	;

%%