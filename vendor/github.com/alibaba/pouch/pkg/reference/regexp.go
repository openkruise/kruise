package reference

import "regexp"

//
// v1 org.opencontainers.image.ref.name:
//
// NOTE: extend separator with "__" here to compatibility with moby
//
// 	ref       ::= component ("/" component)*
// 	component ::= alphanum (separator alphanum)*
// 	alphanum  ::= [A-Za-z0-9]+
// 	separator ::= [-._:@+] | "--" | "__"
//
// In the image-spec, there is no definition about Tag.
//
// But, in fact, the Tag looks like:
//
//	:\w[\w.-]*$
//
var (
	regAlphanum = expression(`[A-Za-z0-9]`)

	regSeparator = expression(`([-._:@+]|--|__)`)

	regComponent = group(
		oneOrMore(regAlphanum),
		zeroOrMore(regSeparator, oneOrMore(regAlphanum)))

	regRef = entire(
		regComponent,
		zeroOrMore(expression("/"), regComponent))

	regTag = expression(`:\w[\w.-]*$`)

	regDigest = expression(`@[A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][[:xdigit:]]{32,}`)
)

// expression converts literal into regexp.
func expression(literal string) *regexp.Regexp {
	return regexp.MustCompile(literal)
}

// concat concats several expressions into one.
func concat(exp ...*regexp.Regexp) *regexp.Regexp {
	s := ""
	for i := range exp {
		s = s + exp[i].String()
	}
	return regexp.MustCompile(s)
}

// entire will match the whole line.
func entire(exp ...*regexp.Regexp) *regexp.Regexp {
	return regexp.MustCompile("^" + concat(exp...).String() + "$")
}

// zeroOrMore will group the expressions and match zero or more times.
func zeroOrMore(exp ...*regexp.Regexp) *regexp.Regexp {
	return regexp.MustCompile(group(concat(exp...)).String() + "*")
}

// zeroOrMore will group the expressions and match one or more times.
func oneOrMore(exp ...*regexp.Regexp) *regexp.Regexp {
	return regexp.MustCompile(group(concat(exp...)).String() + "+")
}

// group defines sub expression.
func group(exp ...*regexp.Regexp) *regexp.Regexp {
	return regexp.MustCompile("(?:" + concat(exp...).String() + ")")
}
