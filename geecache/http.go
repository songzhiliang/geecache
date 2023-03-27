/*
 * @Description:
 * @version:
 * @Author: Steven
 * @Date: 2023-03-27 17:32:51
 */
package geecache

import (
	"fmt"
	"geecache/consistenthash"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	defaultBasePath = "/_geecache/"
	defaultReplicas = 50
)

// HTTPPool implements PeerPicker for a pool of HTTP peers.
type HTTPPool struct {
	// this peer's base URL, e.g. "https://example.net:8000"
	self     string              //记录自己的地址，包括主机名/IP 和端口
	basePath string              //节点间通讯地址的前缀，默认是 /_geecache/。就是分布式集群缓存节点见通信地址前缀
	mu       sync.Mutex          // guards peers and httpGetters
	peers    *consistenthash.Map //一致性哈希算法的 Map
	//映射远程节点与对应的 httpGetter。每一个远程节点对应一个 httpGetter，因为 httpGetter 与远程节点的地址 baseURL 有关。
	httpGetters map[string]*httpGetter // keyed by e.g. "http://10.0.0.2:8008"
}

// NewHTTPPool initializes an HTTP pool of peers.
func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basePath: defaultBasePath,
	}
}

// Log info with server name
func (p *HTTPPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

// ServeHTTP handle all http requests
func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, p.basePath) { //请求的地址不是以basePath开头的，不允许请求！
		panic("HTTPPool serving unexpected path: " + r.URL.Path)
	}
	p.Log("%s %s", r.Method, r.URL.Path)
	// /<basepath>/<groupname>/<key> required
	parts := strings.SplitN(r.URL.Path[len(p.basePath):], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	groupName := parts[0]
	key := parts[1]

	group := GetGroup(groupName) //根据分组名获取该分组实例信息
	if group == nil {
		http.Error(w, "no such group: "+groupName, http.StatusNotFound)
		return
	}

	view, err := group.Get(key) //获取缓存值
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(view.ByteSlice())
}

type httpGetter struct { //实现了peers.go文件中的接口PeerGetter
	baseURL string
}

func (h *httpGetter) Get(group string, key string) ([]byte, error) {
	u := fmt.Sprintf(
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(group), //QueryEscape函数对group进行转码使之可以安全的用在URL查询里。
		url.QueryEscape(key),
	) //u此时是一个url
	res, err := http.Get(u) //向u发送一个GET请求
	if err != nil {
		return nil, err
	}
	defer res.Body.Close() //关闭该请求

	//因为res的状态码如果不是2xx，err一样为nil，所以这里需要判断res.StatusCode != http.StatusOK
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned: %v", res.Status)
	}

	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}

	return bytes, nil
}

// 在ide和编译期验证了httpGetter实现了PeerGetter接口，而不是在使用时，让错误尽早暴露出来，而不是上线后！
var _ PeerGetter = (*httpGetter)(nil)

//还可以这么使用
//var _ PeerGetter = &httpGetter{}

// 一致性hash初始化，生成hash环，为每一个节点配置一个客户端
func (p *HTTPPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistenthash.New(defaultReplicas, nil) //一致性hash map结构体实例化
	p.peers.Add(peers...)                              //生成hash 环
	p.httpGetters = make(map[string]*httpGetter, len(peers))
	for _, peer := range peers { //为每一个节点，初始化一个httpGetter客户端
		p.httpGetters[peer] = &httpGetter{baseURL: peer + p.basePath}
	}
}

// 选择节点，如果缓存没存储在当前请求的节点上，那么返回存储缓存的HTTP客户端，进而可以通过该客户端获取缓存
func (p *HTTPPool) PickPeer(key string) (PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// p.peers.Get(key)：获取缓存key对应的节点，即缓存key的缓存数据应该存在哪个节点上
	//peer != p.self 缓存数据没有存于当前节点上
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		p.Log("Pick peer %s", peer)
		return p.httpGetters[peer], true
	}
	return nil, false
}

var _ PeerPicker = (*HTTPPool)(nil)
