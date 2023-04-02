package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type SelectMode int //不同的负载均衡策略

const (
	RandomSelect     SelectMode = iota // 随机选择
	RoundRobinSelect                   // 轮询
)

// 约定实现服务注册发现需要实现的方法
type Discovery interface {
	Refresh() error                      // 从注册中心更新服务列表
	Update(servers []string) error       //手动更新服务列表
	Get(mode SelectMode) (string, error) //根据负载均衡策略，选择一个服务实例
	GetAll() ([]string, error)           // 返回所有的服务实例
}

// 可以理解为一个实现服务发现注册的插件，实现了接口Discovery
type MultiServersDiscovery struct {
	r       *rand.Rand   // 用来生成随机数的实例
	mu      sync.RWMutex // protect following
	servers []string     //服务列表
	index   int          // 轮询算法需要
}

// NewMultiServerDiscovery creates a MultiServersDiscovery instance
func NewMultiServerDiscovery(servers []string) *MultiServersDiscovery {
	d := &MultiServersDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	d.index = d.r.Intn(math.MaxInt32 - 1) //为了避免每次从 0 开始获取服务，即每次获取到的都是第一个服务。初始化时随机设定一个值。
	return d
}

var _ Discovery = (*MultiServersDiscovery)(nil)

// Refresh doesn't make sense for MultiServersDiscovery, so ignore it
func (d *MultiServersDiscovery) Refresh() error {
	return nil
}

// Update the servers of discovery dynamically if needed
func (d *MultiServersDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	return nil
}

// Get a server according to mode
func (d *MultiServersDiscovery) Get(mode SelectMode) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.servers)
	if n == 0 {
		return "", errors.New("rpc discovery: no available servers")
	}
	switch mode {
	case RandomSelect:
		return d.servers[d.r.Intn(n)], nil
	case RoundRobinSelect:
		s := d.servers[d.index%n] // 第一次获取的服务为d.index%n的结果代表索引位置的服务
		//下一次获取所谓位置为d.index%n+1的服务，但是这里有一个问题
		//如果此时d.index=3,n=4的话，d.index%n+1的结果就编程了4，很明显越界了！
		//针对这个越界的我们就应该将4置为0，可以理解为一个hash环，此时使用的服务就是索引位置为0的服务
		//(d.index + 1) % n，就可以保证不会越界的！
		d.index = (d.index + 1) % n
		return s, nil
	default:
		return "", errors.New("rpc discovery: not supported select mode")
	}
}

// returns all servers in discovery
func (d *MultiServersDiscovery) GetAll() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// return a copy of d.servers
	servers := make([]string, len(d.servers), len(d.servers))
	copy(servers, d.servers)
	return servers, nil
}
