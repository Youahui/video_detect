// DetectionResult 定义 AI 服务返回的检测结果结构
package main

import (
	"fmt"
	"go_client/engine"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	args := os.Args

	_engine, err := engine.NewDetectEngine(args)
	if err != nil {
		fmt.Println("[-] new detect engin err:", err)
		return
	}
	fmt.Println("[+] new detect engin success")

	ch := make(chan os.Signal, 1)

	_engine.Run(ch)

	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
	fmt.Println("[-] detect engine shutdown")
	_engine.Close()

}
