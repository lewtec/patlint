package main

import "os"

func multi() (int, string, error) { return 0, "", nil }
func pair() (int, error)          { return 0, nil }
func one() error                  { return nil }
func three() (int, int, error)    { return 0, 0, nil }
func generic[T any]() (T, error) {
	var z T
	return z, nil
}

type Box[T any] struct{}

func (Box[T]) Get() (T, error) {
	var z T
	return z, nil
}

func hits() {
	a, b, _ := multi()
	x, _ := pair()
	_, _ = pair()
	a, b, _ = multi()
	u, v, _ := three()
	f, _ := os.Open("x")
	_ = one()
	g, _ := generic[int]()
	h, _ := generic[string]()
	var box Box[bool]
	i, _ := box.Get()
	_ = a
	_ = b
	_ = x
	_ = u
	_ = v
	_ = f
	_ = g
	_ = h
	_ = i
}

func misses() {
	// keep error
	a, err := pair()
	_, err2 := pair()
	// blank not last
	_, a2 := pair()
	// not a call RHS
	var z int
	a3, b3, _ := z, z, z
	// generic with error kept
	ok, err3 := generic[int]()
	_ = err
	_ = err2
	_ = a2
	_ = a3
	_ = b3
	_ = z
	_ = a
	_ = ok
	_ = err3
}
