package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"seo/mirror/pkg"
	"strconv"
	"syscall"
	"time"

	"github.com/gookit/slog"
	"github.com/gookit/slog/handler"
	"github.com/gookit/slog/rotatefile"
	"github.com/sgoby/opencc"
)

func main() {

	if len(os.Args) < 2 {
		startCmd()
		return
	}

	switch os.Args[1] {
	case "start":
		cmd := exec.Command(os.Args[0])
		err := cmd.Start()
		if err != nil {
			log.Println("start error:", err.Error())
			return
		}
		pid := fmt.Sprintf("%d", cmd.Process.Pid)
		err = ioutil.WriteFile("pid", []byte(pid), os.ModePerm)
		if err != nil {
			fmt.Println("写入pid文件错误", err.Error())
			cmd.Process.Kill()
			return
		}
		fmt.Println("启动成功", pid)
	case "stop":
		data, err := ioutil.ReadFile("pid")
		if err != nil {
			fmt.Println("read pid error", err.Error())
			return
		}

		pid, err := strconv.Atoi(string(data))
		if err != nil {
			fmt.Println("read pid error", err.Error())
			return
		}
		err = syscall.Kill(pid, syscall.SIGTERM)
		if err != nil {
			fmt.Println("read pid error", err.Error())
			return
		}
		err = os.Remove("pid")
		if err != nil {
			fmt.Println("删除pid文件失败,请手动删除")
		}
		fmt.Println("镜像程序已关闭")

	}
}

func startCmd() {
	rand.Seed(time.Now().UnixNano())
	handler := handler.MustRotateFile("mirror.log", rotatefile.EveryDay, func(c *handler.Config) {
		c.BackupNum = 7
		c.Levels = slog.AllLevels
		c.UseJSON = true
	})
	logger := slog.NewWithHandlers(handler)

	err := pkg.InitTable()
	if err != nil {
		logger.Error("init table error", err.Error())
		return
	}
	appConfig, err := pkg.ParseAppConfig()
	if err != nil {
		logger.Error("parse config error", err.Error())
		return
	}
	//繁体
	s2t, err := opencc.NewOpenCC("s2t")
	if err != nil {
		logger.Error("转繁体功能错误", err.Error())
		return
	}
	dao, err := pkg.NewDao()
	if err != nil {
		logger.Error("数据库错误", err.Error())
		return
	}
	siteConfigs, err := dao.GetAll()
	if err != nil {
		logger.Error("DAO GetAll", err.Error())
		return
	}
	app := pkg.App{
		AppConfig: &appConfig,
		Dao:       dao,
		S2T:       s2t,
		IpList:    pkg.GetIPList(),
		Logger:    logger,
	}
	for _, siteConfig := range siteConfigs {
		err = app.MakeSite(&siteConfig)
		if err != nil {
			logger.Fatal("make Site", err.Error())
			return
		}

	}
	if app.ExpireDate, err = pkg.GetExpireDate(); err != nil {
		logger.Fatal("ExpireDate", err.Error())
		return
	}
	app.Start()
	// 捕获kill的信号
	sigTERM := make(chan os.Signal, 1)
	signal.Notify(sigTERM, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)
	// 收到信号前会一直阻塞

	<-sigTERM
	app.Stop()
	logger.Info("exit")
}
