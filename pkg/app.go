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
	AdminPort     string              `json:"admin_port"`
	CachePath     string              `json:"cache_path"`
	Spider        []string            `json:"spider"`
	GoodSpider    []string            `json:"good_spider"`
	AdminUri      string              `json:"admin_uri"`
	UserAgent     string              `json:"user_agent"`
	GlobalReplace []map[string]string `json:"global_replace"`
	InjectJsPath  string              `json:"inject_js_path"`
	Keywords      []string
	InjectJs      string
	FriendLinks   map[string][]string
}
type App struct {
	*AppConfig
	Dao *SiteConfigDao
	*http.Server
	AdminServer *http.Server
	Sites       sync.Map
	S2T         *opencc.OpenCC
	IpList      []net.IP
}

func (app *App) Start() {
	l, err := net.Listen("tcp", ":"+app.Port)
	if err != nil {
		log.Fatalln(err.Error())
	}
	l = netutil.LimitListener(l, 256*2048)
	app.Server = &http.Server{Handler: app}
	admin := NewAdmin(app)
	app.AdminServer = &http.Server{Handler: admin, Addr: ":" + app.AdminPort}
	go func() {
		if err := app.Serve(l); err != nil {
			log.Fatalln("监听错误" + err.Error())
		}
	}()
	go func() {
		if err := app.AdminServer.ListenAndServe(); err != nil {
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
	err = app.AdminServer.Shutdown(ctx)
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
	if request.URL.Path == app.InjectJsPath {
		writer.Header().Set("Content-Type", "text/javascript;charset=utf-8")
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
	site.Route(writer, request)

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
		appConfig.Keywords = strings.Split(strings.Replace(string(keywordData), "\r", "", -1), "\n")
	}
	//统计js
	js, err := ioutil.ReadFile("config/inject.js")
	if err == nil {
		appConfig.InjectJs = string(js)
	}
	//友情链接文本
	appConfig.FriendLinks = readLinks()

	return appConfig, nil
}
func (app *App) MakeSite(siteConfig *SiteConfig) error {
	return NewSite(siteConfig, app)

}
