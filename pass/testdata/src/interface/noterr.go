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

func _(a A) int {
	if a.b.itf != nil {
		return a.b.itf.Get()
	}
	return 0
}

func _(a A) int {
	switch a.b.itf.(type) {
	case Itf:
		return a.b.itf.Get()
	default:
		return 0
	}
}
