package bytes

import "fmt"

func _(b []byte) {
	if b == nil {
		fmt.Println(b[0])
	}
}
