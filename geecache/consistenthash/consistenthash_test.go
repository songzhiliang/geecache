/*
 * @Description:
 * @version:
 * @Author: Steven
 * @Date: 2023-03-27 21:36:09
 */
package consistenthash

import (
	"strconv"
	"testing"
)

func TestHashing(t *testing.T) {
	hash := New(3, func(key []byte) uint32 {
		i, _ := strconv.Atoi(string(key)) //我们的hash算法，直接使用简单的方式，即将字符串转化为数字，就是对应的hash值
		return uint32(i)
	})

	// Given the above hash function, this will give replicas with "hashes":
	// 2, 4, 6, 12, 14, 16, 22, 24, 26
	hash.Add("6", "4", "2")
	// 2/4/6 三个真实节点，对应的虚拟节点的哈希值是 02/12/22、04/14/24、06/16/26
	//在hash环上的顺序是02、04、06、12、14、16、22、24、26

	// 缓存key分别等于2/11/23/27时，获取虚拟节点计算步骤依次为：
	//key=2时，在hash上找到了下标为0,即第1个hash值为02的虚拟节点
	//key=11时，在hash上找到了下标为3,即第4个hash值为12的虚拟节点
	//key=23时，在hash上找到了下标为7,即第8个hash值为24的虚拟节点
	//key=27时，在hash上没有找到合适的虚拟节点，此时我们选择第一个虚拟节点，即hash指为02的
	//所以2/11/23/27 对应 虚拟节点02/12/24/02
	//而虚拟节点02/12/24/02对应实节点 2/2/4/2
	testCases := map[string]string{
		"2":  "2",
		"11": "2",
		"23": "4",
		"27": "2",
	}

	for k, v := range testCases {
		if hash.Get(k) != v {
			t.Errorf("Asking for %s, should have yielded %s", k, v)
		}
	}

	//在线上运营一段之后，发现我们的分布式缓存，每一个节点的压力有些大，此时我们再加一个节点，来降低每一个节点的压力
	// Adds 8, 18, 28
	hash.Add("8") //对应的虚拟节点为08,18,28

	// 27 should now map to 8.
	testCases["27"] = "8" //此时缓存key为27，就找到了虚拟节点28，对应实节点就是8

	for k, v := range testCases {
		if hash.Get(k) != v {
			t.Errorf("Asking for %s, should have yielded %s", k, v)
		}
	}

}
