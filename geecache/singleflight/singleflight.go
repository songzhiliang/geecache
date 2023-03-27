/*
* @Description:防止缓存击穿，当并发读取缓存时，
* 控制只有一个请求去数据库查询数据！其他请求阻塞，待第一个请求从数据库获取到缓存之后，其他请求结束阻塞，立刻返回缓存数据
* @version:
* @Author: Steven
* @Date: 2023-03-27 23:56:42
 */
package singleflight

import (
	"fmt"
	"sync"
	"time"
)

// call is an in-flight or completed Do call
type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

// Group represents a class of work and forms a namespace in which
// units of work can be executed with duplicate suppression.
type Group struct {
	mu sync.Mutex       // protects m
	m  map[string]*call // lazily initialized
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in, the duplicate caller waits for the
// original to complete and receives the same results.
func (g *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		fmt.Println("if Println:", time.Now().Nanosecond())
		g.mu.Unlock()
		fmt.Println("if Wait:", time.Now().Nanosecond())
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()
	fmt.Println("Sleep:", time.Now().Nanosecond())
	time.Sleep(time.Second * 2)

	c.val, c.err = fn()
	c.wg.Done()
	fmt.Println("Done:", time.Now().Nanosecond())

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}
