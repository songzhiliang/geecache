package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

// hash函数：用户可以自定义hash函数
// Hash maps bytes to uint32
type Hash func(data []byte) uint32

// Map constains all hashed keys
type Map struct {
	hash     Hash           //hash函数
	replicas int            //一个实节点，有几个虚拟节点
	keys     []int          // Hash环上的所有节点，实际上是不包括实节点的，都是实节点的虚拟节点对应的hash值
	hashMap  map[int]string //虚拟节点到实节点的映射，虚拟节点的hash值是key，实节点是value
}

// New creates a Map instance
func New(replicas int, fn Hash) *Map {
	m := &Map{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
	}
	if m.hash == nil {
		m.hash = crc32.ChecksumIEEE //默认采用crc32.ChecksumIEEE
	}
	return m
}

// 生成一个hash环
// keys:所有实节点
// Add adds some keys to the hash.
func (m *Map) Add(keys ...string) {
	for _, key := range keys {
		for i := 0; i < m.replicas; i++ { //针对每一个实节点，遍历计算出该实节点的所有虚拟节点
			//应用New函数实例化map结构体时，传入的hash函数
			//这里我们采用数字编号加上实节点名称组成的字符串作为虚拟节点
			//hash变量就是虚拟节点的hash值
			hash := int(m.hash([]byte(strconv.Itoa(i) + key)))
			m.keys = append(m.keys, hash) //将每一个虚拟节点的hash值，放到hash环里
			m.hashMap[hash] = key         //进行虚拟节点hash值到实节点名称的映射
		}
	}
	//我们现在的hash换m.keys是无序的，我们必须对其进行排序！
	sort.Ints(m.keys) //升序排列
}

// 根据缓存key，获取实节点名称
// Get gets the closest item in the hash to the provided key.
func (m *Map) Get(key string) string {
	if len(m.keys) == 0 { //hash换上没有节点，肯定不用再往下获取节点了
		return ""
	}

	hash := int(m.hash([]byte(key))) //计算缓存key的hash值

	// sort.Search采用二分法，才0<=i<n中搜索满足我们匿名函数条件的最小i,即
	//（m.keys[i] >= hash）=true时，最小的i
	//如果没找到满足条件，即（m.keys[i] >= hash）始终等于false,那么sort.Search返回的是n。
	// 顺时针方向找到大于缓存key的hash值的第一个虚拟节点，在hash环中的下标，即idx
	idx := sort.Search(len(m.keys), func(i int) bool { //选择节点
		return m.keys[i] >= hash
	})
	//但是有一个情况，如果此时hash换上的所有hash值没有大于等于缓存key的hash值的，此时的idx=len(m.keys)
	//这里咱们就定义这种情况时选择的节点为hash环上第一个节点即，m.keys[0]，
	//即当idx=len(m.keys)时，就等同于idx=0。
	//因为任何一个小于len(m.keys)的数i取余len(m.keys)还是为i
	//所以我们使用取余m.keys[idx%len(m.keys)]的形式，就可以获取到hash换上下标为idx的hash值
	//为什么不直接m.keys[idx]，就是因为idx有等于len(m.keys)的情况，这个时候会报越界的错误。
	//因为我们定义了idx=len(m.keys)时，就等同于idx=0,直接一步取余操作即可！

	return m.hashMap[m.keys[idx%len(m.keys)]] //再通过hash值，就可以获取到对应的实节点了
}
