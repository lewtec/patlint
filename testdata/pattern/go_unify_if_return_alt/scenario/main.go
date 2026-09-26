package main

func preferX(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func preferXElse(x, y int) int {
	if x > y {
		return x
	} else {
		return y
	}
}

func preferY(x, y int) int {
	if x > y {
		return y
	}
	return x
}
