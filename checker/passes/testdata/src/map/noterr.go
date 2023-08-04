package nilmap

import "fmt"

func _(m map[string]bool) {
	if m != nil {
		fmt.Println(m["str"])
	}
}
