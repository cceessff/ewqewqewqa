package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"seo/mirror/pkg"
	"strconv"
	"syscall"
	"time"

	"github.com/gookit/slog"
	"github.com/gookit/slog/handler"
	"github.com/gookit/slog/rotatefile"
	"github.com/liuzl/gocc"
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
		err = os.WriteFile("pid", []byte(pid), os.ModePerm)
		if err != nil {
			fmt.Println("写入pid文件错误", err.Error())
			cmd.Process.Kill()
			return
		}
		fmt.Println("启动成功", pid)
	case "stop":
		data, err := os.ReadFile("pid")
		if err != nil {
			fmt.Println("read pid error", err.Error())
			return
		}

		pid, err := strconv.Atoi(string(data))
		if err != nil {
			fmt.Println("read pid error", err.Error())
			return
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			fmt.Println("find process error", err.Error())
			return
		}
		if runtime.GOOS == "windows" {
			err = process.Signal(syscall.SIGKILL)
		} else {
			err = process.Signal(syscall.SIGTERM)
		}
		if err != nil {
			fmt.Println("process.Signal error", err.Error())
			return
		}
		fmt.Println("镜像程序已关闭")

	}
}

func startCmd() {
	rand.Seed(time.Now().UnixNano())
	handle := handler.MustRotateFile("logs/mirror.log", rotatefile.EveryDay, func(c *handler.Config) {
		c.BackupNum = 2
		c.Levels = slog.AllLevels
		c.UseJSON = true
	})
	logger := slog.NewWithHandlers(handle)
	defer logger.Close()
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
	s2t, err := gocc.New("s2t")
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
	ipList, err := pkg.GetIPList()
	if err != nil {
		logger.Error("GetIPList", err.Error())
	}
	app := &pkg.Application{
		AppConfig: &appConfig,
		Dao:       dao,
		S2T:       s2t,
		IpList:    ipList,
		Logger:    logger,
	}
	for i := range siteConfigs {

		err = app.MakeSite(siteConfigs[i])
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
	signal.Notify(sigTERM, syscall.SIGTERM, syscall.Signal(16))
	// 收到信号前会一直阻塞

	<-sigTERM
	app.Stop()
	logger.Info("exit")

}
