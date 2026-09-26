package main

func preferX(x, y int) int {
	return max(x, y)
}

func preferY(x, y int) int {
	if x > y {
		return y
	}
	return x
}
