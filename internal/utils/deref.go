package utils

func Deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func DerefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func DerefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}
