/*
 * @Description:
 * @version:
 * @Author: Steven
 * @Date: 2023-03-27 21:57:08
 */
package geecache

// PeerPicker is the interface that must be implemented to locate
// the peer that owns a specific key.
type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool) //根据传入的 key 选择相应节点 PeerGetter。
}

// PeerGetter is the interface that must be implemented by a peer.
type PeerGetter interface { //就是一个HTTP客户端
	Get(group string, key string) ([]byte, error) //从对应 group 查找缓存值，
}
