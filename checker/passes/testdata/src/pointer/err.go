package pointer

import "fmt"

type fptr = func(int, int) int

func _(ptr *int) { // want "Declared 'ptr'"
	fmt.Println(*ptr) // want "Unsafely used 'ptr'"
}

func _(f fptr) { // want "Declared 'f'"
	f(1, 2) // want "Unsafely used 'f'"
}

type A struct {
	a *int
}

type B struct {
	a A
}

func _(b B) { // want "Declared 'a'"
	fmt.Println(*b.a.a) // want "Unsafely used 'a'"
}
