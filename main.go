/*
 * @Description:
 * @version:
 * @Author: Steven
 * @Date: 2023-03-24 23:00:13
 */
package main

import (
	"flag"
	"fmt"
	"geecache"
	"log"
	"net/http"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

// 创建一个分组缓存实例
func createGroup() *geecache.Group {
	return geecache.NewGroup("scores", 2<<10, geecache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
}

// 启动缓存服务器
func startCacheServer(addr string, addrs []string, gee *geecache.Group) {
	peers := geecache.NewHTTPPool(addr)
	peers.Set(addrs...)      //根据节点addrs，生成了所有节点addrs虚拟节点组成的hash环，并且为每一个实节点分配了一个http客户端！
	gee.RegisterPeers(peers) //将分布式缓存的获取权注册给了gee
	log.Println("geecache is running at", addr)
	log.Fatal(http.ListenAndServe(addr[7:], peers))
}

// 路由为/api
// 为路由/api注册处理程序
func startAPIServer(apiAddr string, gee *geecache.Group) {
	http.Handle("/api", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("key")
			view, err := gee.Get(key) //获取key的缓存值
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(view.ByteSlice())

		}))
	log.Println("fontend server is running at", apiAddr)
	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))

}

func main() {
	var port int
	var api bool
	flag.IntVar(&port, "port", 8001, "Geecache server port") //注册命令行参数格式
	flag.BoolVar(&api, "api", false, "Start a api server?")  //注册命令行参数格式
	flag.Parse()                                             //解析

	apiAddr := "http://localhost:9999" //用户获取缓存数据，通过此url
	addrMap := map[int]string{         //分布式缓存三个节点服务端
		8001: "http://localhost:8001",
		8002: "http://localhost:8002",
		8003: "http://localhost:8003",
	}

	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}

	gee := createGroup()
	if api {
		go startAPIServer(apiAddr, gee) //开启一个为用户查询缓存数据的服务！
	}
	//根据命令行参数，分别使用addrMap下的三个端开启三个服务端，这三个服务用户是看不到的
	startCacheServer(addrMap[port], []string(addrs), gee)
}

type good interface {
	getName() string
}

type car interface {
	getPrice() float64
}
