package xclient

import (
	"context"
	. "geerpc"
	"io"
	"reflect"
	"sync"
)

// 支持负载均衡的客户端
type XClient struct {
	d       Discovery          //一个实现了服务发现注册的工具
	mode    SelectMode         //负责均衡策略
	opt     *Option            //协商协议信息
	mu      sync.Mutex         // protect following
	clients map[string]*Client //Client：一个客户端连接，为了复用连接，减少创建连接的开销。将创建好的节点连接缓存起来，map key就是节点地址
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(d Discovery, mode SelectMode, opt *Option) *XClient {
	return &XClient{d: d, mode: mode, opt: opt, clients: make(map[string]*Client)}
}

func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, client := range xc.clients { //关闭每一个连接
		// I have no idea how to deal with error, just ignore it.
		_ = client.Close() //连接关闭
		delete(xc.clients, key)
	}
	return nil
}

func (xc *XClient) dial(rpcAddr string) (*Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	client, ok := xc.clients[rpcAddr] //检查rpcAddr节点连接是否已经被缓存了
	if ok && !client.IsAvailable() {  //连接被缓存了，但是该节点此时属于不可用状态
		_ = client.Close()          //关闭到该节点的连接
		delete(xc.clients, rpcAddr) //从缓存中删除
		client = nil                //置空，重新创建节点连接
	}
	if client == nil { //需要重新创建节点连接
		var err error
		client, err = XDial(rpcAddr, xc.opt) //拨号,生成客户端到提供服务节点的连接
		if err != nil {
			return nil, err
		}
		xc.clients[rpcAddr] = client //更新缓存
	}
	return client, nil
}

func (xc *XClient) call(rpcAddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := xc.dial(rpcAddr) //向选择的节点进行拨号
	if err != nil {
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply) //向该节点发送请求
}

// Call invokes the named function, waits for it to complete,
// and returns its error status.
// xc will choose a proper server.
func (xc *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := xc.d.Get(xc.mode) //根据指定的负载均衡策略，选择一个可以提供服务的RPC服务节点
	if err != nil {
		return err
	}
	return xc.call(rpcAddr, ctx, serviceMethod, args, reply)
}

// Broadcast invokes the named function for every server registered in discovery
// Broadcast 将请求广播到所有的服务实例，
// 如果任意一个实例发生错误，则返回其中一个错误；如果调用成功，则返回其中一个的结果
func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := xc.d.GetAll() //获取所有服务
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex // protect e and replyDone
	var e error
	replyDone := reply == nil // if reply is nil, don't need to set value
	ctx, cancel := context.WithCancel(ctx)
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) { //并发向所有服务节点发送请求
			defer wg.Done()
			var clonedReply interface{}
			if reply != nil {
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(rpcAddr, ctx, serviceMethod, args, clonedReply)
			mu.Lock()
			if err != nil && e == nil {
				e = err
				cancel() // if any call failed, cancel unfinished calls
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e
}
