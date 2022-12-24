package pkg

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/360EntSecGroup-Skylar/excelize"
)

type AdminModule struct {
	dao      *SiteConfigDao
	app      *App
	adminMux *http.ServeMux
	UserName string
	Password string
	prefix   string
}

func genUserAndPass() (string, string) {
	chars := []rune("abcdefghijklmnopqrstuvwxyz")
	user := ""
	for i := 0; i < 8; i++ {
		user = user + string(chars[rand.Intn(len(chars))])
	}
	chars = []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	pass := ""
	for i := 0; i < 12; i++ {
		pass = pass + string(chars[rand.Intn(len(chars))])
	}
	return user, pass
}
func makeAdminUser() (string, string, error) {
	passBytes, err := ioutil.ReadFile("config/passwd")
	if err != nil || len(passBytes) == 0 {
		userName, password := genUserAndPass()
		err = ioutil.WriteFile("config/passwd", []byte(userName+":"+password), os.ModePerm)
		if err != nil {
			return "", "", errors.New("生成用户文件错误" + err.Error())
		}
		return userName, password, nil

	}
	userAndPass := strings.Split(string(passBytes), ":")
	if len(userAndPass) != 2 {
		return "", "", errors.New("用户文件内容错误")
	}
	return userAndPass[0], userAndPass[1], nil
}

func NewAdmin(app *App) *AdminModule {
	userName, password, err := makeAdminUser()
	if err != nil {
		log.Fatal(err.Error())
	}
	admin := &AdminModule{dao: app.Dao, app: app, prefix: app.AdminUri, UserName: userName, Password: password}
	admin.Initialize()
	return admin
}
func (admin *AdminModule) Initialize() {
	fileHandler := http.FileServer(http.Dir("admin"))
	admin.adminMux = http.NewServeMux()
	prefix := admin.prefix
	admin.adminMux.Handle("/static/", fileHandler)

	admin.adminMux.Handle(prefix+"/login", admin.AuthMiddleware(admin.login))
	admin.adminMux.Handle(prefix, admin.AuthMiddleware(admin.index))
	admin.adminMux.Handle(prefix+"/list", admin.AuthMiddleware(admin.siteList))
	admin.adminMux.Handle(prefix+"/edit", admin.AuthMiddleware(admin.editSite))

	admin.adminMux.Handle(prefix+"/save_config", admin.AuthMiddleware(admin.ConfigSave))
	admin.adminMux.Handle(prefix+"/delete", admin.AuthMiddleware(admin.siteDelete))

	admin.adminMux.Handle(prefix+"/import", admin.AuthMiddleware(admin.siteImport))
	admin.adminMux.Handle(prefix+"/delete_cache", admin.AuthMiddleware(admin.DeleteCache))
	admin.adminMux.Handle(prefix+"/mul_del", admin.AuthMiddleware(admin.MulDel))
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
func (admin *AdminModule) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	admin.adminMux.ServeHTTP(w, r)
}

func (admin *AdminModule) login(writer http.ResponseWriter, request *http.Request) {
	if request.Method == "GET" {
		t := template.Must(template.New("login.html").ParseFiles("admin/login.html"))
		err := t.Execute(writer, map[string]string{"admin_uri": admin.prefix})
		if err != nil {
			log.Println(err.Error())
		}
		return
	}
	err := request.ParseForm()
	if err != nil {
		log.Println(err.Error())
		writer.WriteHeader(404)
		_, _ = writer.Write([]byte(`{"code":5,"msg":"请求出错"}`))
		return
	}
	userName := request.PostFormValue("user_name")
	password := request.PostFormValue("password")
	if userName == "" || password == "" || admin.UserName != userName || admin.Password != password {
		http.Redirect(writer, request, admin.prefix+"/login", http.StatusMovedPermanently)
		return
	}
	sum := sha256.New().Sum([]byte(userName + password))
	loginSign := fmt.Sprintf("%x", sum)
	cookie := &http.Cookie{Name: "login_cert", Value: loginSign, HttpOnly: true, Path: "/"}
	http.SetCookie(writer, cookie)
	http.Redirect(writer, request, admin.prefix, http.StatusMovedPermanently)

}
func (admin *AdminModule) MulDel(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
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
	t, err := template.ParseFiles("admin/admin.html")
	if err != nil {
		log.Println(err.Error())
		return
	}
	err = t.Execute(w, map[string]string{"admin_uri": admin.prefix, "ExpireDate": admin.app.ExpireDate})
	if err != nil {
		log.Println(err.Error())
	}
}
func (admin *AdminModule) forbiddenWords(writer http.ResponseWriter, request *http.Request) {
	if request.Method == "GET" {
		t := template.New("forbidden_words.html")
		t = template.Must(t.ParseFiles("admin/forbidden_words.html"))
		err := t.Execute(writer, map[string]interface{}{"admin_uri": admin.prefix})
		if err != nil {
			log.Println(err.Error())
		}
		return
	}
	err := request.ParseForm()
	if err != nil {
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
		log.Println(err.Error())
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
func (admin *AdminModule) ConfigSave(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":5,"msg":"请求数据出错"}`))
	}

	var needJs = false
	if request.Form.Get("need_js") == "on" {
		needJs = true
	}
	var s2t = false
	if request.Form.Get("s2t") == "on" {
		s2t = true
	}
	var cacheEnable = true
	if request.Form.Get("cache_enable") != "on" {
		cacheEnable = false
	}
	var titleReplace = false
	if request.Form.Get("title_replace") == "on" {
		titleReplace = true
	}

	cacheTimeStr := request.Form.Get("cache_time")
	cacheTime, err := strconv.ParseInt(cacheTimeStr, 10, 64)
	if err != nil || cacheTime == 0 {
		cacheTime = 1440
	}
	id := request.Form.Get("id")
	domain := request.Form.Get("domain")
	u := request.Form.Get("url")
	indexTitle := request.Form.Get("index_title")
	indexKeywords := request.Form.Get("index_keywords")
	indexDescription := request.Form.Get("index_description")
	finds := request.Form.Get("finds")
	replaces := request.Form.Get("replaces")
	h1replace := request.Form.Get("h1replace")
	baiduPushKey := request.Form.Get("baidu_push_key")
	smPushKey := request.Form.Get("sm_push_key")

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
		H1Replace:        h1replace,
		IndexTitle:       indexTitle,
		IndexKeywords:    indexKeywords,
		IndexDescription: indexDescription,
		Finds:            strings.Split(finds, ";"),
		Replaces:         strings.Split(replaces, ";"),
		TitleReplace:     titleReplace,
		NeedJs:           needJs,
		S2t:              s2t,
		CacheEnable:      cacheEnable,
		CacheTime:        cacheTime,
		BaiduPushKey:     baiduPushKey,
		SmPushKey:        smPushKey,
	}

	if siteConfig.Id <= 0 {
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

	if siteConfig.Id <= 0 {
		_, _ = writer.Write([]byte("{\"code\":0,\"action\":\"add\"}"))
		return
	}
	_, _ = writer.Write([]byte("{\"code\":0}"))

}

func (admin *AdminModule) siteDelete(writer http.ResponseWriter, request *http.Request) {
	q := request.URL.Query()
	id := q.Get("id")
	domain := q.Get("domain")
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
	var configs = make([]SiteConfig, 0)
	for k, row := range rows {
		if k <= 0 {
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
		configs = append(configs, siteConfig)
	}
	err = admin.dao.AddMulti(configs)
	if err != nil {
		_, _ = writer.Write([]byte(`{"code":5,"msg":` + err.Error() + `}`))
		return
	}

	for _, data := range configs {
		err := admin.app.MakeSite(&data)
		if err != nil {
			log.Println(err.Error())
		}
	}
	_, _ = writer.Write([]byte("{\"code\":0}"))
}
func (admin *AdminModule) DeleteCache(writer http.ResponseWriter, request *http.Request) {
	q := request.URL.Query()
	domain := q.Get("domain")
	admin.deleteCache(domain)
	_, _ = writer.Write([]byte("{\"code\":0}"))

}
func (admin *AdminModule) deleteCache(domain string) {
	dir := admin.app.CachePath + "/" + domain
	if !isExist(dir) {
		return
	}
	_ = os.RemoveAll(dir)

}
