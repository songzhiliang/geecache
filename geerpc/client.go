/*
 * @Description:
 * @version:
 * @Author: Steven
 * @Date: 2023-03-31 00:01:13
 */
package geerpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"geerpc/codec"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Call represents an active RPC.
type Call struct { //可以理解为呼叫中心，客户端向服务端发送消息，获取响应结果，需要该结构体
	Seq           uint64      //请求编号，每个请求一个唯一的编号,保证客户端服务端消息互通不会错乱！
	ServiceMethod string      // format "<service>.<method>"
	Args          interface{} // arguments to the function
	Reply         interface{} // 服务端响应结果
	Error         error       // if error occurs, it will be set
	Done          chan *Call  // 为了支持异步调用，就是客户端发送了请求，客户端不用一直等着服务端响应，可以并行发送多条请求！
}

// 当调用结束时，会调用 call.done() 通知调用方。
func (call *Call) done() {
	call.Done <- call
}

// Client represents an RPC Client.
// There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
type Client struct {
	cc       codec.Codec      //编解码方式接口
	opt      *Option          //协商协议
	sending  sync.Mutex       // protect following
	header   codec.Header     //header头信息
	mu       sync.Mutex       // protect following
	seq      uint64           //请求编号，每个请求一个唯一的编号
	pending  map[uint64]*Call //存储未处理完的请求，键是编号，值是 Call 实例。
	closing  bool             // user has called Close
	shutdown bool             // server has told us to stop
	//losing 和 shutdown 任意一个值置为 true，则表示 Client 处于不可用的状态，
	//但有些许的差别，closing 是用户主动关闭的，即调用 Close 方法，而 shutdown 置为 true 一般是有错误发生。
}

var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("connection is shut down")

// Close the connection
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	return client.cc.Close()
}

// IsAvailable return true if the client does work
func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.shutdown && !client.closing
}

func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown { //服务不可用了，被关闭了或者出现异常被shtdown
		return 0, ErrShutdown
	}
	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq, nil
}

func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}

// 服务端或客户端发生错误时调用，将 shutdown 设置为 true，且将错误信息通知所有 pending 状态的 call。
func (client *Client) terminateCalls(err error) {
	client.sending.Lock() //不能跟send方法产生数据冲突
	defer client.sending.Unlock()
	client.mu.Lock() //不能跟客户端操作产生冲突
	defer client.mu.Unlock()
	client.shutdown = true //标记服务不可用
	for _, call := range client.pending {
		call.Error = err
		call.done() //将错误信息通知所有 pending 状态的 call。
	}
}

// 接收服务端返回的数据，即接收响应！
// 该协程得先开启，客户端才能向服务端发送其他非协商协议的请求
func (client *Client) receive() {
	var err error
	for err == nil { //出现错误，就将不再接收响应
		var h codec.Header
		//从响应中获取header头信息！
		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}
		call := client.removeCall(h.Seq) //根据 seq，从 client.pending 中移除对应的 call，并返回。
		switch {
		case call == nil: //call 不存在，可能是请求没有发送完整，或者因为其他原因被取消，但是服务端仍旧处理了。
			// it usually means that Write partially failed
			// and call was already removed.
			err = client.cc.ReadBody(nil) //这个时候不需要获取body数据了，gob.Decode(nil)标识丢弃该值
		case h.Error != "": //call 存在，但服务端存现错误
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadBody(nil) //一样将服务端返回的body信息丢弃！如果不丢弃，就会数据错乱
			call.done()                   //调用结束，通知调用方
		default: //call 存在，服务端处理正常，那么需要从 body 中读取 Reply 的值。
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done() //调用结束，通知调用方
		}
	}
	// error occurs, so terminateCalls pending calls
	client.terminateCalls(err)
}

func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType] //获取该编解码方式（opt.CodecType）的构造函数
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	// 将协商协议信息作为数据向服务端发送一次请求，服务端验证该协议
	if err := json.NewEncoder(conn).Encode(opt); err != nil { //验证失败
		log.Println("rpc client: options error: ", err)
		_ = conn.Close() //关闭连接
		return nil, err
	}
	//f(conn)，调用opt.CodecType编解码的构造函数，返回对应结构体实例
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.Codec, opt *Option) *Client {
	client := &Client{
		seq:     1, //请求编号从1开始，0标识无效调用！
		cc:      cc,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive() //开启子协程接收响应
	return client
}

// 支持客户端传入不同的协商协议！
// 使用变长参数实现可选的，用户不传入协商协议，即使用默认的协议！
func parseOptions(opts ...*Option) (*Option, error) {
	// if opts is nil or pass nil as parameter
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt, nil
}

type clientResult struct {
	client *Client
	err    error
}

type newClientFunc func(conn net.Conn, opt *Option) (client *Client, err error)

// 支持超时调用
func dialTimeout(f newClientFunc, network, address string, opts ...*Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	//func net.DialTimeout(network string, address string, timeout time.Duration) (net.Conn, error)
	//连接创建超时，将会返回错误！
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	ch := make(chan clientResult, 1) //有缓冲通道，防止协程泄露！
	go func() {                      //开启协程来创建客户端实例
		defer close(ch)
		client, err := f(conn, opt)
		ch <- clientResult{client: client, err: err} // f(conn, opt)指向完毕，向无缓冲通道里发送数据
	}()
	if opt.ConnectTimeout == 0 { //没有设置超时时间
		result := <-ch
		return result.client, result.err
	}
	select {
	case <-time.After(opt.ConnectTimeout): //如果先执行该case，则说明f(conn, opt)执行超时！返回错误
		return nil, fmt.Errorf("rpc client: connect timeout: expect within %s", opt.ConnectTimeout)
	case result := <-ch:
		return result.client, result.err
	}
}

// 客户端连接rpc服务端需要调用此方法
func Dial(network, address string, opts ...*Option) (client *Client, err error) {
	return dialTimeout(NewClient, network, address, opts...)
}

func (client *Client) send(call *Call) {
	// make sure that the client will send a complete request
	client.sending.Lock()
	defer client.sending.Unlock()

	// register this call.
	seq, err := client.registerCall(call) //注册呼叫中兴
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	// prepare request header
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	//向服务端发送请求数据！
	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call := client.removeCall(seq)
		// call may be nil, it usually means that Write partially failed,
		// client has received the response and handled
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go invokes the function asynchronously.
// It returns the Call structure representing the invocation.
func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	go client.send(call) //异步调用

	return call
}

// 客户端发送请求
// ctx context.Context:传入带超时的上下文
func (client *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := client.Go(serviceMethod, args, reply, make(chan *Call, 1))
	//阻塞直到服务端业务处理完毕，发送完响应结果，客户端receive方法接收完毕，这才会结束阻塞！
	select {
	case <-ctx.Done(): //当先执行该case，则说明超时了！这里超时，就包括：发送报文超时、等待服务端处理超时、接收服务端响应的报文导致超时！
		client.removeCall(call.Seq)
		return errors.New("rpc client: call failed: " + ctx.Err().Error())
	case call := <-call.Done:
		return call.Error
	}
}

// NewHTTPClient new a Client instance via HTTP as transport protocol
func NewHTTPClient(conn net.Conn, opt *Option) (*Client, error) {
	//首先发送CONNECT连接
	_, _ = io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", defaultRPCPath))

	// Require successful HTTP response
	// before switching to RPC protocol.
	//获取该请求http.Request{Method: "CONNECT"}的回复信息
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	//resp=&{Status:200 Connected to Gee RPC StatusCode:200 Proto:HTTP/1.0 ProtoMajor:1 ProtoMinor:0 Header:map[] Body:0xc000206140 ContentLength:-1 TransferEncoding:[] Close:true Uncompressed:false Trailer:map[] Request:0xc00022e100 TLS:<nil>}
	if err == nil && resp.Status == connected { //TCP连接建立成功
		return NewClient(conn, opt) //创建客户端实例，用来发送请求
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	return nil, err
}

// DialHTTP connects to an HTTP RPC server at the specified network address
// listening on the default HTTP RPC path.
func DialHTTP(network, address string, opts ...*Option) (*Client, error) {
	return dialTimeout(NewHTTPClient, network, address, opts...)
}

// XDial calls different functions to connect to a RPC server
// according the first parameter rpcAddr.
// rpcAddr is a general format (protocol@addr) to represent a rpc server
// eg, http@10.0.0.1:7001, tcp@10.0.0.1:9999, unix@/tmp/geerpc.sock
// 支持多种请求协议！
func XDial(rpcAddr string, opts ...*Option) (*Client, error) {
	parts := strings.Split(rpcAddr, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("rpc client err: wrong format '%s', expect protocol@addr", rpcAddr)
	}
	protocol, addr := parts[0], parts[1]
	switch protocol {
	case "http":
		return DialHTTP("tcp", addr, opts...)
	default:
		// tcp, unix or other transport protocol
		return Dial(protocol, addr, opts...)
	}
}
