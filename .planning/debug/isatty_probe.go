package main

import (
	"fmt"

	"golang.org/x/term"
)

func main() {
	fmt.Println(term.IsTerminal(0))
}
