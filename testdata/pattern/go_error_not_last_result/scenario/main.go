package main

// bad: error first
func BadFirst() (error, int) { return nil, 0 }

// bad: error middle
func BadMiddle() (string, error, bool) { return "", nil, false }

// bad: two before error
func BadDeep() (int, string, error, bool) { return 0, "", nil, false }

// bad: named, error not last
func BadNamed() (err error, n int) { return nil, 0 }

// good: error last
func GoodLast() (int, error) { return 0, nil }

// good: only error
func GoodOnly() error { return nil }

// good: named error last
func GoodNamed() (n int, err error) { return 0, nil }

// good: no error
func GoodNone() (int, string) { return 0, "" }

// good: trailing comma, error last
func GoodTrailing() (
	int,
	error,
) {
	return 0, nil
}

// param false positive candidate
func Params(err error, n int) {}

// method
type T struct{}

func (t *T) BadMethod() (error, string)    { return nil, "" }
func (t *T) GoodMethod() (string, error)   { return "", nil }
func (t *T) MethodParams(err error, n int) {}

// function type
var fn func() (error, int)

// interface
type I interface {
	Bad() (error, int)
	Good() (int, error)
}
