package bytes

import "fmt"

func _(b []byte) { // want "Declared 'b'"
	fmt.Println(b[0:1]) // want "Unsafely used 'b'"
}

func _(b []byte) { // want "Declared 'b'"
	fmt.Println(b[0:1]) // want "Unsafely used 'b'"
}
