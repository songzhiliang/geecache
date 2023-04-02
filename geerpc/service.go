/*
 * @Description:
 * @version:
 * @Author: Steven
 * @Date: 2023-03-31 17:34:15
 */
package geerpc

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

// 一个方法的完整信息
type methodType struct {
	method    reflect.Method //方法本身
	ArgType   reflect.Type   //第一个参数的类型
	ReplyType reflect.Type   //第二个参数的类型，因为这里只约定两个参数，所以就定义两个参数即可
	numCalls  uint64         //统计方法调用次数
}

func (m *methodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}

// 初始化参数，返回的是持有参数值的Value,Value持有的类型必须得跟原本类型m.ArgType一样的！
func (m *methodType) newArgv() reflect.Value {
	var argv reflect.Value
	// arg may be a pointer type, or a value type
	if m.ArgType.Kind() == reflect.Ptr { //参数类型如果为指针类型
		//m.ArgType.Elem()去掉指针转化为普通类型，完了再用new申请，new返回的Value,持有的值就是指针，指向的是类型零值
		argv = reflect.New(m.ArgType.Elem())
	} else { //不是指针类型
		//New(m.ArgType)因为返回的Value持有的值是指针，所以为了与原本来行对应上，再调用Elem去掉指针！
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}

// 初始化参数！
func (m *methodType) newReplyv() reflect.Value {
	// reply must be a pointer type
	replyv := reflect.New(m.ReplyType.Elem()) //因为已经约定了m.ReplyType类型必须为指针类型，所以可以直接用此方式初始化！
	switch m.ReplyType.Elem().Kind() {        //m.ReplyType.Elem()去掉指针，比如将类型*int转化为int类型
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem())) //初始化
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0)) //初始化
	}
	return replyv
}

// rpc服务中，服务就是一个结构体，方法就是结构体下的方法
// 所以这里的service就是结构体
type service struct {
	name   string                 //结构体名称
	typ    reflect.Type           //结构体类型
	rcvr   reflect.Value          //结构体实例
	method map[string]*methodType //该结构体所有满足条件的方法
}

// 一个结构体的实例化
func newService(rcvr interface{}) *service {
	s := new(service)
	//func reflect.ValueOf(i any) reflect.Value
	s.rcvr = reflect.ValueOf(rcvr) //通过反射操控一个类型，一般都得先将其转化为reflect.Value或者reflect.type
	//func reflect.TypeOf(i any) reflect.Type
	s.typ = reflect.TypeOf(rcvr)

	//func reflect.Indirect(v reflect.Value) reflect.Value
	//获取参数rcvr的类型名，是实参类型名，而且这里获取到的类型名是不带包名的，也不带指针，因为reflect.Indirect(s.rcvr)返回的是具体值！
	//而s.typ是带有类型名的，还会再包名前面带上指针符号
	s.name = reflect.Indirect(s.rcvr).Type().Name()
	//为啥不直接s.name = s.typ.Name(),因为s.typ.Name()获取不到
	//当rcvr实参类型为指针或者接口，那么可以使用下面方式获取类型名
	//s.name = s.typ.Elem().Name()

	if !ast.IsExported(s.name) { //判断类型是否是可导出的！
		log.Fatalf("rpc server: %s is not a valid service name", s.name)
	}
	s.registerMethods() //注册该结构体所有满足条件的方法
	return s
}

// 注册结构体下的所有满足条件的方法
func (s *service) registerMethods() {
	s.method = make(map[string]*methodType)
	for i := 0; i < s.typ.NumMethod(); i++ { //只能获取到可导出的方法！
		//func (reflect.Type).Method(int) reflect.Method
		method := s.typ.Method(i) //获取第i个方法的信息
		mType := method.Type
		//Foo结构体的Sum方法定义信息：func (f Foo) Sum(args Args, reply *int) error {
		//mType=func(*geerpc.Foo, geerpc.Args, *int) error
		//很明显比定义方式时是定义了两个形参，而mType获取到的有三个形参，有一个是结构体本身！

		//约束方法必须满足的条
		//mType.NumIn()：必须有三个参数，有一个结构体本身，所以需要判断必须有三个参数！
		//mType.NumOut()：必须有一个返回值
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}

		//返回值类型必须为error类型
		//mType.Out(0)=error
		//reflect.TypeOf((error)(nil))=nil,因为error是接口
		//所以reflect.TypeOf((*error)(nil))=*error
		//再把符号*号去掉即可，即再调用Elem即可！
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		//mType.In(1)=geerpc.Args
		//mType.In(2)=*int
		argType, replyType := mType.In(1), mType.In(2)
		//判断类型是否是可导出或者内置类型
		//这里应该还得判断replyType为指针类型吧？我来加一下：|| replyType.Kind() != reflect.Pointer
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) || replyType.Kind() != reflect.Pointer {
			continue
		}
		s.method[method.Name] = &methodType{ //method.Nam方法名
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.name, method.Name)
	}
}

// 判断类型是否是可导出的或者是内置类型
func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

// 调用方法
func (s *service) call(m *methodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1) //原子计数加一，任意时刻，只会有一个协程操作m.numCalls
	f := m.method.Func               //Func是m.method的一个字段：field Func reflect.Value
	//func (reflect.Value).Call(in []reflect.Value) []reflect.Value

	//s.rcvr, argv, replyv：必须得传入三个参数，s.rcvr为结构体本身实例
	returnValues := f.Call([]reflect.Value{s.rcvr, argv, replyv})
	//returnValues[0].Interface()：将returnValues[0]转化为接口类型！
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error) //类型断言，将接口类型errInter断言成error
	}
	return nil
}
