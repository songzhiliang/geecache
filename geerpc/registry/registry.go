package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// 注册中心
type GeeRegistry struct {
	timeout time.Duration          //设定每个服务间隔多久没有发送心跳，即为服务不可用
	mu      sync.Mutex             // protect following
	servers map[string]*ServerItem //服务集合
}

type ServerItem struct {
	Addr  string    //服务具体地址
	start time.Time //服务最新更新时间
}

const (
	defaultPath    = "/_geerpc_/registry"
	defaultTimeout = time.Minute * 5
)

// New create a registry instance with timeout setting
func New(timeout time.Duration) *GeeRegistry {
	return &GeeRegistry{
		servers: make(map[string]*ServerItem),
		timeout: timeout,
	}
}

var DefaultGeeRegister = New(defaultTimeout)

// 向注册中心中注册服务
func (r *GeeRegistry) putServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.servers[addr]
	if s == nil { //当前该服务不在注册中心中，直接添加
		r.servers[addr] = &ServerItem{Addr: addr, start: time.Now()}
	} else { //更新时间
		s.start = time.Now() // if exists, update start time to keep alive
	}
}

// 返回所有可用的服务列表
// 如果有超时的服务，需要删除
func (r *GeeRegistry) aliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr, s := range r.servers {
		//r.timeout == 0:没有设定超时时间
		//s.start.Add(r.timeout).After(time.Now()：当前时间小于s.start+r.timeout，即没过期
		if r.timeout == 0 || s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else { //服务过期了
			delete(r.servers, addr)
		}
	}
	sort.Strings(alive)
	return alive
}

// Runs at /_geerpc_/registry
func (r *GeeRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET": //当请求方法为GET时，即获取所有可用的服务
		// keep it simple, server is in req.Header
		w.Header().Set("X-Geerpc-Servers", strings.Join(r.aliveServers(), ","))
	case "POST": //请求方法为POST时，代表向注册中心注册服务
		// keep it simple, server is in req.Header
		addr := req.Header.Get("X-Geerpc-Server")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// HandleHTTP registers an HTTP handler for GeeRegistry messages on registryPath
func (r *GeeRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path:", registryPath)
}

func HandleHTTP() {
	DefaultGeeRegister.HandleHTTP(defaultPath)
}

// 服务端向注册中心发送心跳，调用该方法
// registry：注册中心地址；addr：服务地址
func Heartbeat(registry, addr string, duration time.Duration) {
	if duration == 0 { //服务端没有设置超时时间，使用默认的超时时间
		// make sure there is enough time to send heart beat
		// before it's removed from registry
		//time.Duration(1)等同于1，就是将1类型转化为time.Duration(1)
		duration = defaultTimeout - time.Duration(1)*time.Minute
	}
	var err error
	err = sendHeartbeat(registry, addr)
	go func() {
		t := time.NewTicker(duration) //每duration分钟发送一次心跳检测，duration必须小于设定的超时时间
		for err == nil {
			<-t.C
			err = sendHeartbeat(registry, addr)
		}
	}()
}

func sendHeartbeat(registry, addr string) error {
	log.Println(addr, "send heart beat to registry", registry)
	httpClient := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil) //创建一个POST请求
	req.Header.Set("X-Geerpc-Server", addr)
	if _, err := httpClient.Do(req); err != nil { //发送请求，更新ServerItem.start
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}
