package nilmap

import "fmt"

func _(m map[string]bool) { // want "Declared 'm'"
	fmt.Println(m["str"]) // want "Unsafely used 'm'"
}
