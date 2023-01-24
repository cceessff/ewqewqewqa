package pkg

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/360EntSecGroup-Skylar/excelize"
)

type AdminModule struct {
	dao      *Dao
	app      *App
	adminMux *http.ServeMux
	UserName string
	Password string
	prefix   string
}
type AdminUser struct {
	UserName string `json:"user_name"`
	Password string `json:"password"`
}

func NewAdmin(app *App) *AdminModule {
	userName, password, err := makeAdminUser()
	if err != nil {
		app.Logger.Fatal("make admin user error", err.Error())
		os.Exit(1)
	}
	admin := &AdminModule{dao: app.Dao, app: app, prefix: app.AdminUri, UserName: userName, Password: password}
	admin.Initialize()
	return admin
}

func (admin *AdminModule) Initialize() {
	fileHandler := http.FileServer(http.Dir("admin"))
	admin.adminMux = http.NewServeMux()
	prefix := admin.prefix
	admin.adminMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, admin.prefix, http.StatusMovedPermanently)
	}))
	admin.adminMux.Handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	admin.adminMux.Handle("/static/", fileHandler)
	admin.adminMux.Handle(prefix+"/login", admin.AuthMiddleware(admin.login))
	admin.adminMux.Handle(prefix, admin.AuthMiddleware(admin.index))
	admin.adminMux.Handle(prefix+"/site", admin.AuthMiddleware(admin.site))
	admin.adminMux.Handle(prefix+"/record", admin.AuthMiddleware(admin.record))
	admin.adminMux.Handle(prefix+"/recordList", admin.AuthMiddleware(admin.recordList))
	admin.adminMux.Handle(prefix+"/del_record", admin.AuthMiddleware(admin.delRecord))
	admin.adminMux.Handle(prefix+"/list", admin.AuthMiddleware(admin.siteList))
	admin.adminMux.Handle(prefix+"/edit", admin.AuthMiddleware(admin.editSite))

	admin.adminMux.Handle(prefix+"/save_config", admin.AuthMiddleware(admin.siteSave))
	admin.adminMux.Handle(prefix+"/delete", admin.AuthMiddleware(admin.siteDelete))

	admin.adminMux.Handle(prefix+"/import", admin.AuthMiddleware(admin.siteImport))
	admin.adminMux.Handle(prefix+"/delete_cache", admin.AuthMiddleware(admin.DeleteCache))
	admin.adminMux.Handle(prefix+"/multi_del", admin.AuthMiddleware(admin.multiDel))
	admin.adminMux.Handle(prefix+"/forbidden_words", admin.AuthMiddleware(admin.forbiddenWords))

}

func (admin *AdminModule) AuthMiddleware(h func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("login_cert")
		sum := sha256.New().Sum([]byte(admin.UserName + admin.Password))
		loginSign := fmt.Sprintf("%x", sum)
		if r.URL.Path != admin.prefix+"/login" && (cookie == nil || cookie.Value != loginSign) {
			http.Redirect(w, r, admin.prefix+"/login", http.StatusMovedPermanently)
			return
		}
		if r.URL.Path == admin.prefix+"/login" && cookie != nil && cookie.Value == loginSign {
			http.Redirect(w, r, admin.prefix, http.StatusMovedPermanently)
			return
		}
		h(w, r)
	})
}
func (admin *AdminModule) login(writer http.ResponseWriter, request *http.Request) {
	if request.Method == "GET" {
		t := template.Must(template.New("login.html").ParseFiles("admin/login.html"))
		err := t.Execute(writer, map[string]string{"admin_uri": admin.prefix})
		if err != nil {
			admin.app.Logger.Error("login template error", err.Error())
		}
		return
	}
	var adminUser AdminUser
	err := json.NewDecoder(request.Body).Decode(&adminUser)
	if err != nil {
		admin.app.Logger.Error("login ParseForm error", err.Error())
		_, _ = writer.Write([]byte(`{"code":5,"msg":"参数错误"}`))
		return
	}

	if adminUser.UserName == "" || adminUser.Password == "" || admin.UserName != adminUser.UserName || admin.Password != adminUser.Password {
		_, _ = writer.Write([]byte(`{"code":4,"msg":"用户名或密码错误"}`))
		return
	}
	sum := sha256.New().Sum([]byte(adminUser.UserName + adminUser.Password))
	loginSign := fmt.Sprintf("%x", sum)
	cookie := &http.Cookie{Name: "login_cert", Value: loginSign, HttpOnly: true, Path: "/"}
	http.SetCookie(writer, cookie)
	_, _ = writer.Write([]byte(`{"code":0,"msg":"登录成功"}`))
}
func (admin *AdminModule) multiDel(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		admin.app.Logger.Error("MulDel ParseForm error", err.Error())
		_, _ = writer.Write([]byte(`{"code":5,"msg":"请求数据出错"}`))
	}
	domains := request.Form.Get("domains")
	if domains == "" {
		_, _ = writer.Write([]byte(`{"code":4,"msg":"域名不能为空"}`))
		return
	}
	domainArr := strings.Split(domains, "\n")
	err = admin.dao.MultiDel(domainArr)
	if err != nil {
		admin.app.Logger.Error("MulDel Dao error", err.Error())
		_, _ = writer.Write([]byte(`{"code":4,"msg":"` + err.Error() + `"}`))
		return
	}
	go func() {
		for _, domain := range domainArr {
			admin.deleteCache(domain)
		}
	}()
	_, _ = writer.Write([]byte(`{"code":0}`))

}
func (admin *AdminModule) index(w http.ResponseWriter, request *http.Request) {
	t, err := template.ParseFiles("admin/index.html")
	if err != nil {
		admin.app.Logger.Error("index template error", err.Error())
		return
	}
	err = t.Execute(w, map[string]string{"admin_uri": admin.prefix, "ExpireDate": admin.app.ExpireDate})
	if err != nil {
		admin.app.Logger.Error("index template error", err.Error())
	}
}
func (admin *AdminModule) site(w http.ResponseWriter, request *http.Request) {
	t, err := template.ParseFiles("admin/site.html")
	if err != nil {
		admin.app.Logger.Error("index template error", err.Error())
		return
	}
	err = t.Execute(w, map[string]string{"admin_uri": admin.prefix, "ExpireDate": admin.app.ExpireDate})
	if err != nil {
		admin.app.Logger.Error("index template error", err.Error())
	}
}
func (admin *AdminModule) record(w http.ResponseWriter, request *http.Request) {
	t, err := template.ParseFiles("admin/record.html")
	if err != nil {
		admin.app.Logger.Error("index template error", err.Error())
		return
	}
	err = t.Execute(w, map[string]string{"admin_uri": admin.prefix})
	if err != nil {
		admin.app.Logger.Error("index template error", err.Error())
	}
}

func (admin *AdminModule) recordList(writer http.ResponseWriter, request *http.Request) {

	params := request.URL.Query()
	var result = make(map[string]interface{})
	domain := params.Get("domain")
	var page int = 1
	var limit int = 50
	var startTime int64 = 0
	var endTime int64 = 0
	if pageParam := params.Get("page"); pageParam != "" {
		page, _ = strconv.Atoi(pageParam)
	}
	if limitParam := params.Get("limit"); limitParam != "" {
		limit, _ = strconv.Atoi(limitParam)
	}
	if startTimeParam := params.Get("start_time"); startTimeParam != "" {
		timeResult, err := time.ParseInLocation("2006-01-02 15:04:05", startTimeParam, time.Local)
		if err == nil {
			startTime = timeResult.Unix()
		}
	}
	if endTimeParam := params.Get("end_time"); endTimeParam != "" {
		timeResult, err := time.ParseInLocation("2006-01-02 15:04:05", endTimeParam, time.Local)
		if err == nil {
			endTime = timeResult.Unix()
		}
	}

	records, err := admin.dao.recordList(domain, startTime, endTime, page, limit)
	if err != nil {
		result["code"] = 2
		result["msg"] = err.Error()
		data, _ := json.Marshal(result)
		_, _ = writer.Write(data)
		return
	}
	count, err := admin.dao.recordCount(domain, startTime, endTime)
	if err != nil {
		result["code"] = 3
		result["msg"] = err.Error()
		data, _ := json.Marshal(result)
		_, _ = writer.Write(data)
		return
	}
	result["code"] = 0
	result["msg"] = ""
	result["count"] = count
	result["data"] = records
	data, _ := json.Marshal(result)
	_, _ = writer.Write(data)

}

func (admin *AdminModule) delRecord(writer http.ResponseWriter, request *http.Request) {
	params := request.URL.Query()
	var result = make(map[string]interface{})
	var startTime int64 = 0
	var endTime int64 = 0
	if startTimeParam := params.Get("start_time"); startTimeParam != "" {
		timeResult, err := time.ParseInLocation("2006-01-02 15:04:05", startTimeParam, time.Local)
		if err == nil {
			startTime = timeResult.Unix()
		}
	}
	if endTimeParam := params.Get("end_time"); endTimeParam != "" {
		timeResult, err := time.ParseInLocation("2006-01-02 15:04:05", endTimeParam, time.Local)
		if err == nil {
			endTime = timeResult.Unix()
		}
	}
	if startTime == 0 || endTime == 0 {
		result["code"] = 2
		result["msg"] = "请输入开始时间和结束时间"
		data, _ := json.Marshal(result)
		_, _ = writer.Write(data)
		return
	}
	err := admin.dao.DelRecord(startTime, endTime)
	if err != nil {
		result["code"] = 3
		result["msg"] = err.Error()
		data, _ := json.Marshal(result)
		_, _ = writer.Write(data)
		return
	}
	result["code"] = 0
	result["msg"] = "删除成功"
	data, _ := json.Marshal(result)
	_, _ = writer.Write(data)

}

func (admin *AdminModule) forbiddenWords(writer http.ResponseWriter, request *http.Request) {
	if request.Method == "GET" {
		t := template.New("forbidden_words.html")
		t = template.Must(t.ParseFiles("admin/forbidden_words.html"))
		err := t.Execute(writer, map[string]interface{}{"admin_uri": admin.prefix})
		if err != nil {
			admin.app.Logger.Error("forbiddenWords template error", err.Error())
		}
		return
	}
	err := request.ParseForm()
	if err != nil {
		admin.app.Logger.Error("forbiddenWords parseform error", err.Error())
		_, _ = writer.Write([]byte(`{"code":5,"msg":"请求参数错误"}`))
	}
	forbiddenWord := request.Form.Get("forbidden_word")
	replaceWord := request.Form.Get("replace_word")
	splitWord := request.Form.Get("split_word")
	if splitWord == "" || forbiddenWord == "" || replaceWord == "" {
		_, _ = writer.Write([]byte(`{"code":2,"msg":"三个参数都要填"}`))
		return
	}
	domainArr, err := admin.dao.ForbiddenWordReplace(forbiddenWord, replaceWord, splitWord)
	if err != nil {
		admin.app.Logger.Error("forbiddenWords ForbiddenWordReplace error", err.Error())
		_, _ = writer.Write([]byte(`{"code":3,"msg":"` + err.Error() + `"}`))
		return
	}
	for _, value := range domainArr {
		da := strings.Split(value, "##")
		admin.deleteCache(da[0])
		un, ok := admin.app.Sites.Load(da[0])
		if ok {
			site := un.(*Site)
			site.IndexTitle = da[1]
		}
	}
	_, _ = writer.Write([]byte(`{"code":0,"msg":""}`))

}

func (admin *AdminModule) editSite(writer http.ResponseWriter, request *http.Request) {
	v := request.URL.Query()
	s := v.Get("url")
	t := template.New("edit.html")
	t.Funcs(template.FuncMap{"join": strings.Join})
	t = template.Must(t.ParseFiles("admin/edit.html"))
	siteConfig := SiteConfig{}
	var err error
	if s != "" {
		siteConfig, err = admin.dao.GetOne(s)
		if err != nil {
			_ = t.Execute(writer, map[string]string{"error": err.Error()})
			return
		}
	}
	err = t.Execute(writer, map[string]interface{}{"proxy_config": siteConfig, "admin_uri": admin.prefix})
	if err != nil {
		admin.app.Logger.Error("editSite template error", err.Error())
	}

}

func (admin *AdminModule) siteList(writer http.ResponseWriter, request *http.Request) {
	v := request.URL.Query()
	page := v.Get("page")
	limit := v.Get("limit")
	domain := v.Get("domain")
	var result = make(map[string]interface{})
	p, err := strconv.Atoi(page)
	if err != nil {
		result["code"] = 1
		result["msg"] = err.Error()
		data, _ := json.Marshal(result)
		_, _ = writer.Write(data)
		return
	}
	size, err := strconv.Atoi(limit)
	if err != nil {
		result["code"] = 4
		result["msg"] = err.Error()
		data, _ := json.Marshal(result)
		_, _ = writer.Write(data)
		return
	}
	if domain != "" {
		proxy, err := admin.dao.GetOne(domain)
		if err != nil {
			result["code"] = 2
			result["msg"] = err.Error()
			data, _ := json.Marshal(result)
			_, _ = writer.Write(data)
			return
		}
		result["code"] = 0
		result["msg"] = ""
		result["count"] = 1
		result["data"] = []SiteConfig{proxy}
		data, _ := json.Marshal(result)
		_, _ = writer.Write(data)
		return

	}
	proxys, err := admin.dao.GetByPage(p, size)
	if err != nil {
		result["code"] = 2
		result["msg"] = err.Error()
		data, _ := json.Marshal(result)
		_, _ = writer.Write(data)
		return
	}
	count, err := admin.dao.Count()
	if err != nil {
		result["code"] = 3
		result["msg"] = err.Error()
		data, _ := json.Marshal(result)
		_, _ = writer.Write(data)
		return
	}
	result["code"] = 0
	result["msg"] = ""
	result["count"] = count
	result["data"] = proxys
	data, _ := json.Marshal(result)
	_, _ = writer.Write(data)

}
func (admin *AdminModule) siteSave(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":5,"msg":"请求数据出错"}`))
	}

	id := request.Form.Get("id")
	domain := request.Form.Get("domain")
	u := request.Form.Get("url")
	cacheTime, err := strconv.ParseInt(request.Form.Get("cache_time"), 10, 64)
	if err != nil || cacheTime == 0 {
		cacheTime = 1440
	}
	i, err := strconv.Atoi(id)
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":2,"msg":` + err.Error() + `}`))
		return
	}
	if _, err := url.Parse(u); err != nil {
		_, _ = writer.Write([]byte(`{"code":3,"msg":` + err.Error() + `}`))
		return
	}
	if _, err := url.Parse(domain); err != nil {
		_, _ = writer.Write([]byte(`{"code":4,"msg":` + err.Error() + `}`))
		return
	}
	siteConfig := SiteConfig{
		Id:               i,
		Domain:           domain,
		Url:              u,
		H1Replace:        request.Form.Get("h1replace"),
		IndexTitle:       request.Form.Get("index_title"),
		IndexKeywords:    request.Form.Get("index_keywords"),
		IndexDescription: request.Form.Get("index_description"),
		Finds:            strings.Split(request.Form.Get("finds"), ";"),
		Replaces:         strings.Split(request.Form.Get("replaces"), ";"),
		TitleReplace:     request.Form.Get("title_replace") == "on",
		NeedJs:           request.Form.Get("need_js") == "on",
		S2t:              request.Form.Get("s2t") == "on",
		CacheEnable:      request.Form.Get("cache_enable") == "on",
		CacheTime:        cacheTime,
		BaiduPushKey:     request.Form.Get("baidu_push_key"),
		SmPushKey:        request.Form.Get("sm_push_key"),
	}

	if siteConfig.Id == 0 {
		err = admin.dao.addOne(siteConfig)
	} else {
		err = admin.dao.UpdateById(siteConfig)
	}
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":1,"msg":` + err.Error() + `}`))
		return
	}
	err = admin.app.MakeSite(&siteConfig)
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":2,"msg":` + err.Error() + `}`))
		return
	}

	if siteConfig.Id == 0 {
		_, _ = writer.Write([]byte("{\"code\":0,\"action\":\"add\"}"))
		return
	}
	_, _ = writer.Write([]byte("{\"code\":0}"))

}

func (admin *AdminModule) siteDelete(writer http.ResponseWriter, request *http.Request) {
	q := request.URL.Query()
	id := q.Get("id")
	domain := q.Get("domain")
	if domain == "" {
		_, _ = writer.Write([]byte(`{"code":1,"msg":"域名不能为空"}`))
		return
	}
	i, err := strconv.Atoi(id)
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":1,"msg":` + err.Error() + `}`))
		return
	}
	err = admin.dao.DeleteOne(i)
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":1,"msg":` + err.Error() + `}`))
		return
	}
	admin.app.Sites.Delete(domain)
	admin.deleteCache(domain)
	_, _ = writer.Write([]byte("{\"code\":0}"))

}

func (admin *AdminModule) siteImport(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":5,"msg":` + err.Error() + `}`))
		return
	}
	mf, _, err := request.FormFile("file")
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":1,"msg":` + err.Error() + `}`))
		return
	}

	f, err := excelize.OpenReader(mf)
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":1,"msg":` + err.Error() + `}`))
		return
	}
	rows := f.GetRows("Sheet1")
	var configs = make([]*SiteConfig, 0)
	for k, row := range rows {
		if k == 0 {
			continue
		}
		if _, err := url.Parse(row[1]); err != nil {
			_, _ = writer.Write([]byte(`{"code":3,"msg":` + err.Error() + `}`))
			return
		}
		if _, err := url.Parse(row[0]); err != nil {
			_, _ = writer.Write([]byte(`{"code":4,"msg":` + err.Error() + `}`))
			return
		}
		cacheTime, err := strconv.ParseInt(row[11], 10, 64)
		if err != nil || cacheTime == 0 {
			cacheTime = 1440
		}
		var siteConfig SiteConfig = SiteConfig{
			Domain:           row[0],
			Url:              row[1],
			IndexTitle:       row[2],
			IndexKeywords:    row[3],
			IndexDescription: row[4],
			Finds:            strings.Split(row[5], ";"),
			Replaces:         strings.Split(row[6], ";"),
			H1Replace:        row[7],
			NeedJs:           row[8] != "0" && strings.ToLower(row[8]) != "false",
			S2t:              row[9] != "0" && strings.ToLower(row[9]) != "false",
			TitleReplace:     row[10] != "0" && strings.ToLower(row[10]) != "false",
			CacheEnable:      true,
			CacheTime:        cacheTime,
			BaiduPushKey:     row[12],
			SmPushKey:        row[13],
		}
		configs = append(configs, &siteConfig)
	}
	err = admin.dao.AddMulti(configs)
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":5,"msg":` + err.Error() + `}`))
		return
	}

	for i := range configs {
		err := admin.app.MakeSite(configs[i])
		if err != nil {
			admin.app.Logger.Error(err.Error())
		}
	}
	_, _ = writer.Write([]byte("{\"code\":0}"))
}
func (admin *AdminModule) DeleteCache(writer http.ResponseWriter, request *http.Request) {
	q := request.URL.Query()
	domain := q.Get("domain")
	if domain == "" {
		_, _ = writer.Write([]byte(`{"code":5,"msg":"域名不能为空"}`))
		return
	}
	admin.deleteCache(domain)
	_, _ = writer.Write([]byte(`{"code":0}`))

}
func (admin *AdminModule) deleteCache(domain string) {
	if domain == "" {
		return
	}
	dir := admin.app.CachePath + "/" + domain
	if !isExist(dir) {
		return
	}
	_ = os.RemoveAll(dir)

}
