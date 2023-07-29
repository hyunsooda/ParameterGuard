package pointer

import "fmt"

func _(ptr *int) {
	if ptr != nil {
		fmt.Println(*ptr)
	}
}

func _(f fptr) {
	if f != nil {
		f(1, 2)
	}
}

func _(b B) {
	if b.a.a != nil {
		fmt.Println(*b.a.a)
	}
}
