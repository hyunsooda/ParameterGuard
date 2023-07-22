package nilstruct

import "fmt"

type S struct {
	a int
	b int
}

func _(s *S) { // want "Declared 's'"
	fmt.Println(s.a) // want "Unsafely used 's'"
}
