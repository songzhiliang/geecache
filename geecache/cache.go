/*
 * @Description:支持缓存并发读写
 * @version:
 * @Author: Steven
 * @Date: 2023-03-27 15:41:25
 */
package geecache

import (
	"geecache/lru"
	"sync"
)

type cache struct {
	mu         sync.Mutex //分布式锁
	lru        *lru.Cache //存储缓存的源，即最底层负责缓存更新，淘汰策略的！
	cacheBytes int64      //缓存大小
}

func (c *cache) add(key string, value ByteView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		c.lru = lru.New(c.cacheBytes, nil) //延迟初始化，即在第一次调用add方法时，才进行初始化
	}
	c.lru.Add(key, value)
}

func (c *cache) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		return
	}

	if v, ok := c.lru.Get(key); ok {
		return v.(ByteView), ok
	}

	return
}
