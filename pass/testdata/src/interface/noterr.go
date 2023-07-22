package itf

func _(i Itf) int {
	if i != nil {
		return i.Get()
	}
	return 0
}

func _(i Itf) int {
	switch i.(type) {
	case Itf:
		return i.Get()
	default:
		return 0
	}
}
