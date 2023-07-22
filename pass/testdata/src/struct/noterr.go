package nilstruct

import "fmt"

func _(s *S) {
	if s != nil {
		fmt.Println(s.a)
	}
}
