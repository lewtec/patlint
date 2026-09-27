package main

import "strings"

func head(s string) string {
	parts := strings.SplitN(s, "=", 2)
	return parts[0]
}

func keep(s string) []string {
	return strings.SplitN(s, "=", 3)
}
