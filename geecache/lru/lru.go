package lru

import (
	"container/list" //双向链表
)

// Cache is a LRU cache. It is not safe for concurrent access.
type Cache struct {
	maxBytes int64                    //最大可以使用的内存
	nbytes   int64                    //当前已经使用的内存
	ll       *list.List               //双向链表
	cache    map[string]*list.Element //缓存键值对map，值为双向链表中某一个元素的指针
	// optional and executed when an entry is purged.
	OnEvicted func(key string, value Value) //某条记录被移除时的回调函数，可以为 nil
}

// 双向链表节点的数据类型，
type entry struct {
	key   string //在链表中仍保存每个值对应的 key 的好处在于，淘汰队首节点时，需要用 key 从字典中删除对应的映射
	value Value
}

// Value use Len to count how many bytes it takes
type Value interface {
	Len() int //返回值所占用的内存大小,如：值是字符串，因为可以len(字符串)，所以这个值类型就实现了Value接口，就可以调用起len方法
}

// New is the Constructor of Cache
func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

// Add adds a value to the cache.
func (c *Cache) Add(key string, value Value) {
	if ele, ok := c.cache[key]; ok { //缓存存在
		c.ll.MoveToFront(ele) //将该缓存移到到双向链表的队首
		//此时ele.Value的值为&entry{key, value}，但是类型是any,即空接口
		//所以需要使用类型断言
		//将ele.Value断言成*entry类型，这里肯定会断言成功，所以不用使用kv,ok:= ele.Value.(*entry)的判断形式
		kv := ele.Value.(*entry)
		//虽然断言之后的 kv是指针类型，但是不用*(kv).value，因为go底层会将kv.value转化成*(kv).value

		//一开始我以为这里会有问题，即如果int64(value.Len())小于int64(kv.value.Len())，那相减之后不就是负数了嘛。
		//后来才反应过来c.nbytes +=一个负数，那不就是c.nbytes-=这个数嘛！
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value //将新值覆给kv.value
	} else { //缓存不存在
		//&entry{key, value}作为值，插入到链表队首
		//插入之后ele.Value就是&entry{key, value}
		ele := c.ll.PushFront(&entry{key, value})
		c.cache[key] = ele
		c.nbytes += int64(len(key)) + int64(value.Len())
	}
	for c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest()
	}
}

// Get look ups a key's value
func (c *Cache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok { //从字典中找到对应的双向链表的节点
		c.ll.MoveToFront(ele) //将链表中的节点 ele 移动到队首
		kv := ele.Value.(*entry)
		return kv.value, true
	}
	return
}

// RemoveOldest removes the oldest item
func (c *Cache) RemoveOldest() {
	ele := c.ll.Back() //返回链表队尾元素
	if ele != nil {
		c.ll.Remove(ele)                                       //从链表中删除该元素ele
		kv := ele.Value.(*entry)                               //虽然从链表中删除了元素ele,但是在这列elde还是存在的，依然可以ele.Value
		delete(c.cache, kv.key)                                //从缓存键值对c.cache中删除，key为kv.key的键值对
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len()) //计算淘汰该缓存之后，以使用的内存
		if c.OnEvicted != nil {
			c.OnEvicted(kv.key, kv.value) //删除该缓存之后，调用回调函数
		}
	}
}

func (c *Cache) Len() int {
	return c.ll.Len()
}
