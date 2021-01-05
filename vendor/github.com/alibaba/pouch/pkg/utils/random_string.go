package utils

import (
	"math/rand"
	"time"
)

const (
	letterBytes     = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789"
	stringSeparator = "_"
)

// RandString is a thread/goroutine safe solution to generate a random string
// of a fixed length
func RandString(n int, prefix, suffix string) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[r.Intn(len(letterBytes))]
	}
	randStr := string(b)
	if prefix != "" {
		randStr = prefix + stringSeparator + randStr
	}
	if suffix != "" {
		randStr += stringSeparator + suffix
	}
	return randStr
}
