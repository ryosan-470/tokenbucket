package testutils

import "math/rand"

func GenerateRandomIntRange(min, max int) int {
	if min >= max {
		return min
	}
	return min + rand.Intn(max-min)
}
