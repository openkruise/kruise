package util

// Filter filters elements in a slice and returns the filtered slice
// T: the type of elements in the slice
// s: the slice to be filtered
// remove: a function that returns true if the element should be removed, false otherwise
// returns: a new slice containing only the elements that were not removed
func Filter[T any](s []T, remove func(T) bool) []T {
	// Use two-pointer technique to filter elements in-place
	w := 0
	for _, v := range s {
		if !remove(v) {
			s[w] = v
			w++
		}
	}
	return s[:w]
}
