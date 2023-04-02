package main

import "fmt"

type str struct {
	sl []int
}

func test(s str) []int {
	return s.sl
}

func main() {
	var sa str = str{sl: make([]int, 0, 10)}
	sa.sl = append(sa.sl, 10)
	sl1 := test(sa)
	fmt.Println(sa, "|", sl1)
	sl1 = append(sl1, 20)
	fmt.Println(sa, "|", sl1)
}
