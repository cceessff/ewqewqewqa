package pkg

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gookit/slog"
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
	ExpireDate  string
	Logger      *slog.Logger
}

func (app *App) Start() {
	l, err := net.Listen("tcp", ":"+app.Port)
	if err != nil {
		app.Logger.Fatalln("net listen", err.Error())
		return
	}
	l = netutil.LimitListener(l, 256*2048)
	app.Server = &http.Server{Handler: app}
	admin := NewAdmin(app)
	app.AdminServer = &http.Server{Handler: admin, Addr: ":" + app.AdminPort}
	go func() {
		if err := app.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			app.Logger.Fatalln("监听错误" + err.Error())
			os.Exit(1)
		}
	}()
	go func() {
		if err := app.AdminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			app.Logger.Fatalln("监听错误" + err.Error())
			os.Exit(1)
		}
	}()

}
func (app *App) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	err := app.Shutdown(ctx)
	if err != nil {
		app.Logger.Error("shutdown error" + err.Error())
	}
	err = app.AdminServer.Shutdown(ctx)
	if err != nil {
		app.Logger.Error("shutdown error" + err.Error())
	}
	defer cancel()
}
func (app *App) ServeHTTP(writer http.ResponseWriter, request *http.Request) {

	if authErr := app.Auth(); authErr != nil {
		_, _ = writer.Write([]byte(authErr.Error()))
		return
	}
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
func (app *App) Auth() error {
	if expire, err := time.Parse("2006-01-02", app.ExpireDate); err != nil || expire.Unix() < time.Now().Unix() {
		return errors.New("已到期，请重新续期")
	}
	return nil
}

func (app *App) MakeSite(siteConfig *SiteConfig) error {
	return NewSite(siteConfig, app)

}
