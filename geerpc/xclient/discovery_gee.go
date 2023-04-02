package xclient

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type GeeRegistryDiscovery struct {
	*MultiServersDiscovery               //匿名嵌套
	registry               string        //注册中心的地址
	timeout                time.Duration //需要过多久重新从注册中心获取可用的服务
	lastUpdate             time.Time     //记录上次从注册中心获取可用服务的时间
}

const defaultUpdateTimeout = time.Second * 10 //默认每10秒冲注册中心获取可用的服务

func NewGeeRegistryDiscovery(registerAddr string, timeout time.Duration) *GeeRegistryDiscovery {
	if timeout == 0 {
		timeout = defaultUpdateTimeout
	}
	d := &GeeRegistryDiscovery{
		MultiServersDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		registry:              registerAddr,
		timeout:               timeout,
	}
	return d
}

// 更新服务列表
func (d *GeeRegistryDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	d.lastUpdate = time.Now()
	return nil
}

// 过了约定时间之后 重新从注册中心获取可用的服务
func (d *GeeRegistryDiscovery) Refresh() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lastUpdate.Add(d.timeout).After(time.Now()) { //没有超过规定时间
		return nil
	}
	log.Println("rpc registry: refresh servers from registry", d.registry)
	resp, err := http.Get(d.registry) //从注册中心拉取可用的服务
	if err != nil {
		log.Println("rpc registry refresh err:", err)
		return err
	}
	//func strings.Split(s string, sep string) []string
	servers := strings.Split(resp.Header.Get("X-Geerpc-Servers"), ",")
	d.servers = make([]string, 0, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server) != "" {
			d.servers = append(d.servers, strings.TrimSpace(server))
		}
	}
	d.lastUpdate = time.Now()
	return nil
}

func (d *GeeRegistryDiscovery) Get(mode SelectMode) (string, error) {
	if err := d.Refresh(); err != nil { //先更新可用的服务
		return "", err
	}
	return d.MultiServersDiscovery.Get(mode) //负载均衡，基于策略mode,选择一个可用的服务
}

func (d *GeeRegistryDiscovery) GetAll() ([]string, error) {
	if err := d.Refresh(); err != nil {
		return nil, err
	}
	return d.MultiServersDiscovery.GetAll() //获取所有可用的服务
}
