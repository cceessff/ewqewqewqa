package main

import (
	"log"
	"math/rand"
	"os"
	"os/signal"
	"seo/mirror/pkg"
	"syscall"
	"time"

	"github.com/sgoby/opencc"
)

func init() {
	rand.Seed(time.Now().UnixNano())
	err := pkg.InitTable()
	if err != nil {
		log.Fatal("init table error", err.Error())
	}
}

func main() {
	appConfig, err := pkg.ParseAppConfig()
	if err != nil {
		log.Fatal("parse config error", err.Error())
	}
	//繁体
	s2t, err := opencc.NewOpenCC("s2t")
	if err != nil {
		log.Fatal("转繁体功能错误" + err.Error())
	}
	app := pkg.App{
		AppConfig: &appConfig,
		Dao:       new(pkg.SiteConfigDao),
		S2T:       s2t,
		IpList:    pkg.GetIPList(),
	}
	app.Start()
	// 捕获kill的信号
	sigTERM := make(chan os.Signal)
	signal.Notify(sigTERM, syscall.SIGTERM)
	// 收到信号前会一直阻塞
	<-sigTERM
	app.Stop()
}
