package feishu

// 连接层与扫码注册之间的解耦点。
//
// 扫码成功后凭据变了,长连接必须重连;但 register.go 只做协议,
// 不该知道连接层长什么样(它连 SDK 都不 import)。于是这里留一个
// 包级钩子:main 里由连接层注册,handler 只负责喊一声。
//
// 同一个钩子在设置页手动改 App ID/Secret 后也会被调用。

import "sync"

var (
	hookMu          sync.Mutex
	reconfigureHook func()
)

// SetReconfigureHook 由连接层在初始化时调用。传 nil 表示关闭。
func SetReconfigureHook(fn func()) {
	hookMu.Lock()
	reconfigureHook = fn
	hookMu.Unlock()
}

// Reconfigure 通知连接层按最新配置重建连接。没有连接层时是空操作,
// 这样扫码/设置页在只跑单测的环境里也能正常工作。
func Reconfigure() {
	hookMu.Lock()
	fn := reconfigureHook
	hookMu.Unlock()
	if fn != nil {
		fn()
	}
}
