package nilstruct

import "fmt"

func _(s *S) {
	if s != nil {
		fmt.Println(s.a)
	}
}

func _(c C) {
	if c.b != nil {
		fmt.Println(c.b.a)
	}
}

func _(c C) {
	if c.b != nil && c.b.a != nil {
		fmt.Println(c.b.a.n)
	}
}
