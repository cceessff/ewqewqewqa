package pkg

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sgoby/opencc"
	"golang.org/x/net/netutil"
)

type AppConfig struct {
	Port          string              `json:"port"`
	CachePath     string              `json:"cache_path"`
	Spider        []string            `json:"spider"`
	GoodSpider    []string            `json:"good_spider"`
	AdminUri      string              `json:"admin_uri"`
	UserAgent     string              `json:"user_agent"`
	GlobalReplace []map[string]string `json:"global_replace"`
	Keywords      []string
	InjectJs      string
	FriendLinks   map[string][]string
}
type App struct {
	*AppConfig
	Dao *SiteConfigDao
	*http.Server
	Sites sync.Map
	S2T   *opencc.OpenCC
}

func (app *App) Start() {
	l, err := net.Listen("tcp", ":"+app.Port)
	if err != nil {
		log.Fatalln(err.Error())
	}
	l = netutil.LimitListener(l, 256*2048)
	app.Server = &http.Server{Handler: app}
	go func() {
		if err := app.Serve(l); err != nil {
			log.Fatalln("监听错误" + err.Error())
		}
	}()

}
func (app *App) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	err := app.Shutdown(ctx)
	if err != nil {
		log.Println("shutdown error" + err.Error())
	}
	defer cancel()
}
func (app *App) ServeHTTP(writer http.ResponseWriter, request *http.Request) {

	// if authErr := m.auth(); authErr != nil {
	// 	_, _ = writer.Write([]byte(authErr.Error()))
	// 	return
	// }
	if request.URL.Path == "/abcd/abcd.js" {
		writer.Write([]byte(app.InjectJs))
		return
	}
	host := GetHost(request)
	item, ok := app.Sites.Load(host)
	if !ok {
		_, _ = writer.Write([]byte("未找到该代理域名，请检查配置 " + host))
		return
	}
	site := item.(*Site)
	site.ServeHTTP(writer, request)

}

func ParseAppConfig() (AppConfig, error) {
	var appConfig AppConfig
	data, err := ioutil.ReadFile("config.json")
	if err != nil {
		return appConfig, err
	}

	err = json.Unmarshal(data, &appConfig)
	if err != nil {
		return appConfig, err
	}
	//关键字文件
	keywordData, err := ioutil.ReadFile("config/keywords.txt")
	if err == nil && len(keywordData) > 0 {
		keywordStr := strings.Replace(string(keywordData), "\r", "", -1)
		appConfig.Keywords = strings.Split(keywordStr, "\n")
	}
	//统计js
	js, err := ioutil.ReadFile("config/inject.js")
	if err == nil {
		appConfig.InjectJs = string(js)
	}
	//友情链接文本
	linkData, err := ioutil.ReadFile("config/links.txt")
	if err == nil && len(linkData) > 0 {
		linkLines := strings.Split(strings.Replace(string(linkData), "\r", "", -1), "\n")
		for _, line := range linkLines {
			linkArr := strings.Split(line, "||")
			if len(linkArr) < 2 {
				continue
			}
			appConfig.FriendLinks[linkArr[0]] = linkArr[1:]
		}
	}

	return appConfig, nil
}
