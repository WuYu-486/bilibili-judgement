//go:build windows

package main

import "syscall"

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
)

func init() {
	// 程序日志输出为 UTF-8, 将 Windows 控制台输出代码页切到 UTF-8(65001),
	// 否则账号名/投票文本等中文在默认 GBK(936) 控制台上会显示乱码。
	// 在非控制台(管道/重定向)场景下调用无副作用。
	_, _, _ = procSetConsoleOutputCP.Call(65001)
}
