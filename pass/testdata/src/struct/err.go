package nilstruct

import "fmt"

type S struct {
	a int
	b int
}

func _(s *S) { // want "Declared 's'"
	fmt.Println(s.a) // want "Unsafely used 's'"
}

type A struct {
	n *int
}

type B struct {
	a *A
}

type C struct {
	b *B
	f float32
}

// Test report and compiled program's reports are different each other.
// The compiled one is more informative. We don't put much of effort in the test report
func _(c C) { // want "Declared 'b'"
	fmt.Println(c.b.a) // want "Unsafely used 'b'"
}

func _(c C) { // want "Declared 'b'" "Declared 'a'"
	fmt.Println(c.b.a.n) // want "Unsafely used 'a'" "Unsafely used 'b'"
}
