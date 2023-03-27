/*
 * @Description:对外暴露方法
 * @version:
 * @Author: Steven
 * @Date: 2023-03-27 15:42:00
 */
package geecache

import (
	"fmt"
	"geecache/singleflight"
	"log"
	"sync"
)

// 当我们要获取的数据，在缓存里还没有的时候，我们就需要从数据源获取数据，而不同的缓存数据对应的数据源是不一样的！
// 我们不可能针对不同数据源做多种的配置！这个获取数据应该由每一位程序开发者自己来决定，更加方便维护扩展！
// 这个时候就需要一个回调函数了：
// 比如我们通过实例了的变量new获取缓存，当缓存获取不到的时候，我们就应该继续通过new来调用回调函数获取缓存值
// 通过回调函数获取到缓存之后，再将该缓存值写入保存缓存的数据库里！
// 下面的Getter就是一个告诉程序员，你编写的回调函数应该最从的规则！即必须实现Getter接口
// A Getter loads data for a key.
type Getter interface {
	Get(key string) ([]byte, error) //key:缓存值对应的key
}

// 接口型函数：实现了接口Getter
// 定义一个函数类型 F，并且实现接口 A 的方法，然后在这个方法中调用自己。
// 这是 Go 语言中将其他函数（参数返回值定义与 F 一致）转换为接口 A 的常用技巧。
// A GetterFunc implements Getter with a function.
type GetterFunc func(key string) ([]byte, error)

// Get implements Getter interface function
func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key) //f为一个匿名函数或者具名函数，都可以通过Get方法，实现调用该函数
}

// 一个group可以理解为一个缓存命名空间，就是分组的概念
// 比如学生、老师、家长，就可以是不同的缓存分组
// A Group is a cache namespace and associated data loaded spread over
type Group struct {
	name      string     //分组名
	getter    Getter     //未获取缓存时，通过该字段，进行调用回调函数来获取缓存值，进而更新到缓存数据库里
	mainCache cache      //一套并发缓存数据库的维护，通过该字段可以从缓存数据库获取缓存更新缓存
	peers     PeerPicker //可以通过这，从分布式缓存系统获取缓存数据
	loader    *singleflight.Group
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group) //保存着每一个缓存分组名到具体缓存Group结构体实例的映射
)

// name:缓存分组名
// cacheBytes:该缓存可以使用的内存空间大小
// getter:回调函数
// NewGroup create a new instance of Group
func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cacheBytes: cacheBytes},
		loader:    &singleflight.Group{},
	}
	groups[name] = g
	return g
}

// GetGroup returns the named group previously created with NewGroup, or
// nil if there's no such group.
func GetGroup(name string) *Group {
	mu.RLock()
	g := groups[name]
	mu.RUnlock()
	return g
}

// 这里就看出来ByteView结构体的作用了！
// Get value for a key from cache
func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
	}

	//从缓存数据库获取缓存值
	if v, ok := g.mainCache.get(key); ok {
		log.Println("[GeeCache] hit")
		return v, nil
	}
	//没获取到，获取缓存值
	return g.load(key)
}

// 获取缓存值：缓存数据源有多种源头，比如从本地获取，从远程获取
// 这里暂时定义，直接从本地获取！
func (g *Group) load(key string) (value ByteView, err error) {
	if g.peers != nil { //配置了远程分布式缓存获取算法
		if peer, ok := g.peers.PickPeer(key); ok { //peer是一个从分布式缓存系统获取缓存数据的http客户端
			if value, err = g.getFromPeer(peer, key); err == nil {
				return value, nil
			}
			log.Println("[GeeCache] Failed to get from peer", err)
		}
	}

	return g.getLocally(key)
	viewi, err := g.loader.Do(key, func() (interface{}, error) {
		if g.peers != nil { //配置了远程分布式缓存获取算法
			if peer, ok := g.peers.PickPeer(key); ok { //peer是一个从分布式缓存系统获取缓存数据的http客户端
				if value, err = g.getFromPeer(peer, key); err == nil {
					return value, nil
				}
				log.Println("[GeeCache] Failed to get from peer", err)
			}
		}

		return g.getLocally(key)
	})

	if err == nil {
		return viewi.(ByteView), nil
	}
	return
}

// 从远程分布式缓存获取缓存
func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: bytes}, nil
}

// 从本地获取缓存数据
func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key) //调用NewGroup函数第三个参数的匿名函数
	if err != nil {
		return ByteView{}, err

	}
	value := ByteView{b: cloneBytes(bytes)} //将缓存值保存到结构体ByteView中
	g.populateCache(key, value)
	return value, nil
}

func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}

// 注册分布式缓存操作权到该分组下
func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
}
