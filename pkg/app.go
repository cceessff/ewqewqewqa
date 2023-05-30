package pkg

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gookit/slog"
	"github.com/liuzl/gocc"
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
	AdDomains     map[string]bool
}
type App struct {
	*AppConfig
	Dao *Dao
	*http.Server
	AdminServer *http.Server
	Sites       sync.Map
	S2T         *gocc.OpenCC
	IpList      []net.IP
	ExpireDate  string
	Logger      *slog.Logger
	RecordChann chan *Record
	Finish      chan int
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
	app.AdminServer = &http.Server{Handler: admin.adminMux, Addr: ":" + app.AdminPort}
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

	go func() {
		for record := range app.RecordChann {
			err := app.Dao.AddRecord(record)
			if err != nil {
				app.Logger.Error(err.Error())
			}
		}
		app.Finish <- 1

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
	close(app.RecordChann)
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
	site, err := app.querySite(host)
	if err != nil {
		writer.Write([]byte(err.Error()))
		return
	}
	if site.Scheme == "" {
		site.Scheme = request.Header.Get("scheme")
	}
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

func (app *App) querySite(host string) (*Site, error) {
	hostParts := strings.Split(host, ".")
	if len(hostParts) == 1 {
		return nil, errors.New("站点不存在，请检查配置")
	}
	item, ok := app.Sites.Load(host)
	if ok {
		return item.(*Site), nil
	}
	return app.querySite(strings.Join(hostParts[1:], "."))
}
func (app *App) AddRecord(domain, path, userAgent string) {

	record := &Record{
		Domain:      domain,
		Path:        path,
		UserAgent:   userAgent,
		Spider:      app.getSpiderName(userAgent),
		CreatedTime: time.Now().Unix(),
	}
	app.RecordChann <- record
}
func (app *App) getSpiderName(userAgent string) string {
	userAgent = strings.ToLower(userAgent)
	for _, value := range app.Spider {
		spider := strings.ToLower(value)
		if strings.Contains(userAgent, spider) {
			return spider
		}
	}
	return ""
}
