package util

func SliceToSet[A comparable](s []A) map[A]bool {
	m := make(map[A]bool, len(s))
	for _, e := range s {
		m[e] = true
	}
	return m
}

func MapValuesToSet[K, V comparable](m map[K]V) map[V]bool {
	s := make(map[V]bool, len(m))
	for _, v := range m {
		s[v] = true
	}
	return s
}

func MapKeysToSlice[K comparable, V any](m map[K]V) []K {
	s := make([]K, len(m))
	i := 0
	for k := range m {
		s[i] = k
		i++
	}
	return s
}
