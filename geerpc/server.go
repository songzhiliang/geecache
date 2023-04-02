/*
 * @Description:
 * @version:
 * @Author: Steven
 * @Date: 2023-03-29 23:07:57
 */
package geerpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"geerpc/codec"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

const MagicNumber = 0x3bef5c

// Option承载着通信双方(客户端和服务端)协商信息，就是通过该信息来识别出对应的内容！再通过该内容去编解码header和body信息
type Option struct {
	MagicNumber    int           // 可以认为是密钥，从Option解析出来的MagicNumber必须等于常量MagicNumber的值
	CodecType      codec.Type    // 约定的编解码方式,通过该编解码方式编解码header和body信息
	ConnectTimeout time.Duration // 连接超时设置。0代表没有设置超时
	HandleTimeout  time.Duration //服务端进行业务处理超时设置！0代表没有设置超时
}

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType, //默认编解码方式就是GOB
	ConnectTimeout: time.Second * 10,
}

// 服务端设计
// Server represents an RPC Server.
type Server struct {
	serviceMap sync.Map //服务名到服务实例的映射
}

// 将单个服务注册到服务端
func (server *Server) Register(rcvr interface{}) error {
	s := newService(rcvr) //单个服务的实例
	//如果key=s.name不在集合server.serviceMap，就将key=s.name,value=s存储到集合里,LoadOrStore方法返回的第一个参数就是s,第二个参数dup为false
	//如果key=s.name在集合里，那么第二个参数dup值为true
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return errors.New("rpc: service already defined: " + s.name) //报告服务已经被注册过的服务！
	}
	return nil
}

// 方便直接利用默认服务来注册
func Register(rcvr interface{}) error { return DefaultServer.Register(rcvr) }

// 服务寻找，即结构体寻找
// serviceMethod 格式 结构体名字.方法名字
func (server *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 { //不满足格式
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	//获取想要调用的服务名和方法名
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	//func (m* Map) Load(key interface{}) (value interface{}, ok bool)
	svci, ok := server.serviceMap.Load(serviceName)
	if !ok { //服务名没有被注册！
		err = errors.New("rpc server: can't find service " + serviceName)
		return
	}
	svc = svci.(*service)          //类型断言，将接口类型svci断言成指针service类型
	mtype = svc.method[methodName] //从服务实例中获取方法
	if mtype == nil {
		err = errors.New("rpc server: can't find method " + methodName)
	}
	return
}

// NewServer returns a new Server.
func NewServer() *Server {
	return &Server{}
}

// DefaultServer is the default instance of *Server.
var DefaultServer = NewServer()

// Accept accepts connections on the listener and serves requests
// for each incoming connection.
func (server *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept() //这里等待客户端连接,没有连接的话，会一直在这里阻塞
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		//log.Println("Accept go before")
		go server.ServeConn(conn) //接到连接之后，开启一个协程去处理该连接
	}
}

// 暂时来看，就是一个简化操作，只有main.go文件才调用了该函数
func Accept(lis net.Listener) { DefaultServer.Accept(lis) }

// 这个函数可以理解为，协议认证
// 即校验客户端发送的协议信息是否正确
func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()
	var opt Option
	//log.Println("json.NewDecoder before")
	//从conn中反序列取出协商信息即Option信息
	//这里有学问：
	//func json.NewDecoder(r io.Reader) *json.Decoder：参数得是实现了io.Reader的类型，而conn就是实现了io.Reader！
	//函数的用途：创建一个从r读取内容并将内容解码为json对象的解码器
	//那么json.NewEncoder(conn)的意思就是从conn读取内容，也就是通过conn.read方法读取内容，
	//读取的什么内容，当然是客户端发送的请求数据，当没有向客户端发送请求数据之前conn.read一直是阻塞的。
	//只建立了连接，而没有发送任何数据，read就一直阻塞，你发送的数据字节为0，也一样会阻塞！
	//只有调用了conn.Write方法，向连接中写入数据，才会结束阻塞！可以理解为无缓冲通道！
	//当结束阻塞，即读取到内容之后，调用.Decode(&opt)方法，将内容解码之后，赋值给opt变量
	//此时这里就会结束阻塞！
	//因为我们在main函数中，首先向连接发送的是协商协议内容，即默认的geerpc.DefaultOption
	//所以这里解码之后opt的值就是geerpc.DefaultOption
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error: ", err)
		return
	}
	//log.Println("json.NewDecoder after")
	if opt.MagicNumber != MagicNumber { //密钥不正确
		log.Printf("rpc server: invalid magic number %x", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType] //获取对应编解码的构造函数
	if f == nil {                             //该编解码没有被注册
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}
	//f(conn)就是调用opt.CodecType加密方式的构造函数，返回的是该加密结构体的实例指针
	//结构体实例指针作为参数，调用server.serveCodec方法
	//协议信息校验完毕，开始等待用户发送的header和body信息，进而进行处理
	server.serveCodec(f(conn))
}

// invalidRequest is a placeholder for response argv when error occurs
var invalidRequest = struct{}{}

// 这个方法，是最终接收客户端发送的header和body信息，并且进行响应！
func (server *Server) serveCodec(cc codec.Codec) {
	sending := new(sync.Mutex) // make sure to send a complete response
	wg := new(sync.WaitGroup)  // 保证次函数里的协程都执行完毕，再退出该函数
	//因为一次连接中，可以有多个请求，即多个 request header 和 request body
	//因此这里使用了 for 无限制地等待请求的到来，直到发生错误（例如连接被关闭，接收到的报文有问题等）
	//一次连接会进入这个函数一次，一次连接里的多次请求也只会进入这个函数一次！
	//log.Println("serveCodec in")
	for {
		req, err := server.readRequest(cc) //读取请求
		if err != nil {
			if req == nil {
				break // it's not possible to recover, so close the connection
			}
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending) //出现错误，请求处理直接完毕，进而回复请求
			continue
		}
		wg.Add(1)
		cn := make(chan struct{})
		go server.handleRequest(cc, req, sending, wg, cn) //处理请求
		//业务代码处理请求超时控制
		timeout := time.Second * 2
		go func() {
			select {
			case <-time.After(timeout): //业务代码请求处理超时！
				req.h.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
				server.sendResponse(cc, req.h, invalidRequest, sending)
			case <-cn:
			}
		}()
	}
	wg.Wait()      //等待所有协程处理完毕
	_ = cc.Close() //处理完毕之后，关闭连接
}

// request stores all information of a call
type request struct {
	h            *codec.Header // header of request
	argv, replyv reflect.Value // argv and replyv of request
	mtype        *methodType   //请求的方法
	svc          *service      //请求的服务
}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

// 处理请求
func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := server.readRequestHeader(cc) //获取Header信息
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	//服务查找
	req.svc, req.mtype, err = server.findService(h.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.argv = req.mtype.newArgv()     //初始化参数
	req.replyv = req.mtype.newReplyv() //初始化参数

	// 获取参数req.argv所指向的内存地址，就是指向其值的指针，而不是req.argv的内存地址
	//将指针赋值给argvi，这样修改了argvi也就修改了req.argv所指向的值
	//假设req.argv是指针类型，Interface方法是返回req.argv所持有的指针值，保存到返回值接口里！
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr { //如果req.argv不是指针类型
		// req.argv.Addr()：获取指向req.argv其值的指针！
		argvi = req.argv.Addr().Interface()
	}
	//此时argvi和req.argv指向的是用一个内存地址，修改了argvi也就修改了req.argv
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read body err:", err)
		return req, err
	}

	return req, nil
}

// 处理请求是并发的，但是回复请求的报文必须是逐个发送的，并发容易导致多个回复报文交织在一起，客户端无法解析。在这里使用锁(sending)保证
func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

// 使用了协程来处理多次请求，所以处理的多个请求是并发执行的！
func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, cn chan struct{}) {
	// TODO, should call registered rpc methods to get the right replyv
	// day 1, just print argv and send a hello message
	defer func() {
		wg.Done()
		close(cn)
	}()

	// req.svc是服务实例；req.mtype是方法；req.argv方法的参数；req.replyv服务端响应结果保存在这里
	//call:调用服务者的call方法，进行方法的调用
	err := req.svc.call(req.mtype, req.argv, req.replyv)
	if err != nil {
		req.h.Error = err.Error() //出错，将错误信息放到响应的header头里！
		server.sendResponse(cc, req.h, invalidRequest, sending)
		return
	}

	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
}

const (
	connected        = "200 Connected to Gee RPC"
	defaultRPCPath   = "/_geeprc_"
	defaultDebugPath = "/debug/geerpc"
)

// ServeHTTP implements an http.Handler that answers RPC requests.
func (server *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	//我们需要CONNECT来做代理服务，通过代理可创建一个TCP双向隧道！
	//比如当客户端发送的请求是Https请求，由于https请求都是加密的！代理服务器并不知道要向哪个地址那个端口来发送请求！
	//所以这个时候就需要浏览器先以明文的形式发送向代理服务器发送一个CONNECT请求，告诉代理服务器我要请求的地址和端口号
	//代理服务器接收到这个请求后，会在对应端口与目标站点建立一个 TCP 连接，
	//连接建立成功后返回 HTTP 200 状态码告诉浏览器与该站点的加密通道已经完成。
	//接下来代理服务器仅需透传浏览器和服务器之间的加密数据包即可，代理服务器无需解析 HTTPS 报文
	//认证成功之后，浏览器和服务器就可以通过代理服务器来做媒介达到消息的互通！

	if req.Method != "CONNECT" { //请求动作不是CONNECT
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "405 must CONNECT\n")
		return
	}
	//上面的CONNECT就是应用层，我们将应用层去掉，下一层就是传输层

	//func (http.Hijacker).Hijack() (net.Conn, *bufio.ReadWriter, error)
	//正常来说，一个连接发过来，经过ServeHTTP处理之后，会由http扩展包里conn结构体下的finishRequest方法关闭连接
	//所以我们为了不关闭连接，这里需要从连接中提取出传输层tcp协议的连接，进而实现双向通道，RPC调用就是基于传输层TCP的！
	//将w类型断言成http.Hijacker，完了调用Hijack方法，进而接管连接，建立tcp双向通道，不会在响应结果之后关闭该连接！进而实现客户端和服务端继续用该连接来通信，比如实现在线聊天！

	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
		return
	}
	//io.WriteString：无缓冲立刻向conn中写入内容
	_, _ = io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n") //发送响应内容，客户端会接到此内同
	server.ServeConn(conn)
}

// 路由跟处理器的映射注册
func (server *Server) HandleHTTP() {
	http.Handle(defaultRPCPath, server)              //注册RPC服务，defaultRPCPath作为路由
	http.Handle(defaultDebugPath, debugHTTP{server}) //注册debug服务，defaultDebugPath作为路由
	log.Println("rpc server debug path:", defaultDebugPath)
}

// 为默认的服务，注册http.Handler
func HandleHTTP() {
	DefaultServer.HandleHTTP()
}
