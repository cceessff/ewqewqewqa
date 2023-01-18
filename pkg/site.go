package pkg

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type Site struct {
	*SiteConfig
	*httputil.ReverseProxy
	Schema    string
	app       *App
	CachePath string
}
type CustomResponse struct {
	StatusCode int
	// Body is the content of the Response
	Body []byte
	// Headers contains the Response's HTTP headers
	Header http.Header
}

func NewSite(siteConfig *SiteConfig, app *App) error {
	u, err := url.Parse(siteConfig.Url)
	if err != nil {
		return err
	}

	siteConfig.IndexTitle = HtmlEntities(siteConfig.IndexTitle)
	siteConfig.IndexKeywords = HtmlEntities(siteConfig.IndexKeywords)
	siteConfig.IndexDescription = HtmlEntities(siteConfig.IndexDescription)
	for _, item := range app.GlobalReplace {
		siteConfig.Replaces = append(siteConfig.Replaces, item["replace"])
		siteConfig.Finds = append(siteConfig.Finds, item["needle"])
	}
	for i, replace := range siteConfig.Replaces {
		siteConfig.Replaces[i] = HtmlEntities(replace)
	}

	proxy := newProxy(u, app.IpList)
	site := &Site{SiteConfig: siteConfig, ReverseProxy: proxy, CachePath: app.CachePath, app: app}
	proxy.ModifyResponse = func(r *http.Response) error {
		return site.ModifyResponse(r)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		site.ErrorHandler(w, r, err)
	}
	app.Sites.Store(siteConfig.Domain, site)
	return nil
}

func (site *Site) Route(writer http.ResponseWriter, request *http.Request) {
	ua := request.UserAgent()
	host := GetHost(request)
	request = request.WithContext(context.WithValue(context.WithValue(request.Context(), "Origin-Ua", ua), "HOST", host))
	if site.isCrawler(ua) && !site.isGoodCrawler(ua) { //如果是蜘蛛但不是好蜘蛛
		writer.WriteHeader(404)
		_, _ = writer.Write([]byte("页面未找到"))
		return
	}

	cacheKey := host + request.URL.Path + request.URL.RawQuery
	if site.CacheEnable {
		if cacheResponse := site.getCache(cacheKey, host, false); cacheResponse != nil {
			for key, values := range cacheResponse.Header {
				writer.Header()[key] = values
			}
			var content = cacheResponse.Body

			if strings.Contains(strings.ToLower(cacheResponse.Header.Get("Content-Type")), "html") {
				isSpider := site.isCrawler(ua)
				if isSpider {
					site.app.AddRecord(host, request.URL.Path, ua)
				}
				content = site.injectJs(content, isIndexPage(request.URL), isSpider)
			}
			contentLength := int64(len(content))
			writer.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
			if cacheResponse.StatusCode != 0 {
				writer.WriteHeader(cacheResponse.StatusCode)
			} else {
				writer.WriteHeader(200)
			}
			_, err := writer.Write(content)
			if err != nil {
				site.app.Logger.Error("写出错误：", err.Error(), request.URL)
			}
			return
		}

	}
	if site.app.UserAgent != "" {
		request.Header.Set("User-Agent", site.app.UserAgent)
	}
	site.ServeHTTP(writer, request)

}
func (site *Site) ModifyResponse(response *http.Response) error {
	host := response.Request.Context().Value("HOST").(string)
	if response.StatusCode == 301 || response.StatusCode == 302 {
		return site.handleRedirectResponse(response, host)
	}
	//host := GetHost(response.Request)

	cacheKey := host + response.Request.URL.Path + response.Request.URL.RawQuery
	if response.StatusCode == 200 {
		content, err := site.readResponse(response)
		if err != nil {
			return err
		}
		contentType := strings.ToLower(response.Header.Get("Content-Type"))

		if strings.Contains(contentType, "text/html") {
			return site.handleHtmlResponse(content, response, contentType, host)
		} else if strings.Contains(contentType, "css") || strings.Contains(contentType, "javascript") {
			content = GBk2UTF8(content, contentType)
			contentStr := site.replaceHost(string(content), host)
			content = []byte(contentStr)
			site.setCache(cacheKey, response, content, host)
			site.wrapResponseBody(response, content)
			return nil

		}
		site.setCache(cacheKey, response, content, host)
		site.wrapResponseBody(response, content)
		return nil

	}
	if response.StatusCode > 400 && response.StatusCode < 500 {
		content := []byte("访问的页面不存在")
		site.setCache(cacheKey, response, content, host)
		site.wrapResponseBody(response, content)
	}
	return nil
}

func (site *Site) handleRedirectResponse(response *http.Response, host string) error {
	redirectUrl, err := response.Request.URL.Parse(response.Header.Get("Location"))
	if err != nil {
		return err
	}
	redirectUrl.Host = host
	redirectUrl.Scheme = site.Schema
	// if !isIndexPage(redirectUrl) {
	// 	site.EncodeUrl(redirectUrl)
	// }
	response.Header.Set("Location", redirectUrl.String())
	return nil
}
func (site *Site) handleHtmlNode(node *html.Node, host string, isIndexPage bool, replacedH1 *bool) {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode, html.CommentNode, html.RawNode:
			c.Data = site.transformText(c.Data, host)
		case html.ElementNode:
			if c.Data == "a" {
				site.transformANode(c, host)
			}
			if c.Data == "link" {
				site.transformLinkNode(c, host)
			}
			if c.Data == "title" {
				site.transformTitleNode(c, isIndexPage)
			}
			if c.Data == "script" {
				site.transformScriptNode(c)
			}
			if c.Data == "meta" {
				site.transformMetaNode(c, isIndexPage)
			}
			if c.Data == "body" {
				nodes, err := html.ParseFragment(strings.NewReader(RandHtml()), c)
				if err == nil {
					c.InsertBefore(nodes[0], c.FirstChild)
				}
			}
			if c.Data == "h1" && c.FirstChild != nil && c.FirstChild.Type == html.TextNode && site.H1Replace != "" {
				c.FirstChild.Data = site.H1Replace
				*replacedH1 = true
			}
			for i, attr := range c.Attr {
				if attr.Key == "href" || attr.Key == "src" {
					c.Attr[i].Val = site.replaceHost(attr.Val, host)
				}
				if attr.Key == "title" || attr.Key == "alt" || attr.Key == "value" || attr.Key == "placeholder" {
					for index, find := range site.Finds {
						tag := fmt.Sprintf("{{replace:%d}}", index)
						attr.Val = strings.ReplaceAll(attr.Val, find, tag)
					}
					c.Attr[i].Val = attr.Val
					if site.S2t {
						c.Attr[i].Val, _ = site.app.S2T.ConvertText(attr.Val)
					}
				}
			}

		}
		site.handleHtmlNode(c, host, isIndexPage, replacedH1)
	}
}
func (site *Site) transformMetaNode(node *html.Node, isIndexPage bool) {
	content := ""
	for i, attr := range node.Attr {
		if attr.Key == "name" && attr.Val == "keywords" && isIndexPage {
			content = "{{index_keywords}}"
			break
		}
		if attr.Key == "name" && attr.Val == "description" && isIndexPage {
			content = "{{index_description}}"
			break
		}
		if strings.ToLower(attr.Key) == "http-equiv" && strings.ToLower(attr.Val) == "content-type" {
			content = "text/html; charset=UTF-8"
			break
		}
		if attr.Key == "charset" {
			node.Attr[i].Val = "UTF-8"
		}
	}
	if content == "" {
		return
	}
	for i, attr := range node.Attr {
		if attr.Key == "content" {
			node.Attr[i].Val = content
		}
	}

}
func (site *Site) transformScriptNode(node *html.Node) {
	if site.NeedJs {
		return
	}
	if node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
		node.FirstChild.Data = ""
	}
	for i, attr := range node.Attr {
		if attr.Key == "src" {
			node.Attr[i].Val = ""
			break
		}
	}

}
func (site *Site) transformText(text string, host string) string {
	for index, find := range site.Finds {
		tag := fmt.Sprintf("{{replace:%d}}", index)
		text = strings.ReplaceAll(text, find, tag)
	}
	text = site.replaceHost(text, host)
	if site.S2t {
		text, _ = site.app.S2T.ConvertText(text)
	}
	return text
}
func (site *Site) transformLinkNode(node *html.Node, host string) {
	var isAlternate bool = false
	for _, attr := range node.Attr {
		if attr.Key == "rel" && attr.Val == "alternate" {
			isAlternate = true
			break
		}
	}
	if !isAlternate {
		return
	}
	for i, attr := range node.Attr {
		if attr.Key == "href" {
			node.Attr[i].Val = "//" + host
			break
		}
	}
}
func (site *Site) transformANode(node *html.Node, host string) {
	ou, _ := url.Parse(site.Url)
	for i, attr := range node.Attr {
		if attr.Key != "href" || attr.Val == "" {
			continue
		}

		u, _ := ou.Parse(attr.Val)
		if u == nil {
			break
		}
		if u.Host == ou.Host {
			u.Scheme = site.Schema
			u.Host = host
			node.Attr[i].Val = u.String()
			break
		}
		if u.Path == "" {
			//path为空，是友情链接，全部删除
			node = nil
			break
		}
		//不是友情链接，只删除链接，不删除文字
		node.Attr[i].Val = "#"
		break
	}
}
func (site *Site) parseTemplateTags(content []byte) []byte {
	contentStr := string(content)
	contentStr = strings.ReplaceAll(contentStr, "{{index_title}}", site.IndexTitle)
	contentStr = strings.ReplaceAll(contentStr, "{{index_keywords}}", site.IndexKeywords)
	contentStr = strings.ReplaceAll(contentStr, "{{index_description}}", site.IndexDescription)
	keywordRegexp, _ := regexp.Compile(`\{\{keyword:(\d+)\}\}`)
	keywordTags := keywordRegexp.FindAllStringSubmatch(contentStr, -1)
	for _, keywordTag := range keywordTags {
		index, err := strconv.Atoi(keywordTag[1])
		if err != nil {
			continue
		}
		contentStr = strings.ReplaceAll(contentStr, keywordTag[0], site.app.Keywords[index])
	}
	replaceRegexp, _ := regexp.Compile(`\{\{replace:(\d+)\}\}`)
	replaceTags := replaceRegexp.FindAllStringSubmatch(contentStr, -1)
	for _, replaceTag := range replaceTags {
		index, err := strconv.Atoi(replaceTag[1])
		if err != nil {
			continue
		}
		contentStr = strings.ReplaceAll(contentStr, replaceTag[0], site.Replaces[index])
	}
	return []byte(contentStr)
}
func (site *Site) handleHtmlResponse(content []byte, response *http.Response, contentType string, host string) error {
	isIndexPage := isIndexPage(response.Request.URL)
	content = site.handleHtmlContent(content, host, contentType, isIndexPage)
	content = site.parseTemplateTags(content)
	cacheKey := host + response.Request.URL.Path + response.Request.URL.RawQuery
	site.setCache(cacheKey, response, content, host)
	originUa := response.Request.Context().Value("Origin-Ua").(string)
	isSpider := site.isCrawler(originUa)
	content = site.injectJs(content, isIndexPage, isSpider)
	site.wrapResponseBody(response, content)
	if isSpider {
		site.app.AddRecord(host, response.Request.URL.Path, originUa)
	}
	return nil

}
func (site *Site) handleHtmlContent(content []byte, host string, contentType string, isIndexPage bool) []byte {
	content = GBk2UTF8(content, contentType)
	document, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		site.app.Logger.Error("html parse error", err.Error())
		return content
	}
	var replacedH1 bool = false
	for c := document.FirstChild; c != nil; c = c.NextSibling {
		site.handleHtmlNode(c, host, isIndexPage, &replacedH1)
		if !replacedH1 && c.FirstChild != nil && c.FirstChild.NextSibling != nil && site.H1Replace != "" {
			c.FirstChild.NextSibling.InsertBefore(&html.Node{
				Type: html.ElementNode,
				Data: "h1",
				FirstChild: &html.Node{
					Type: html.TextNode,
					Data: site.H1Replace,
				},
			}, c.FirstChild.NextSibling.FirstChild)
		}
	}

	var buf bytes.Buffer
	err = html.Render(&buf, document)
	if err != nil {
		site.app.Logger.Error("html render error", err.Error())
		return content
	}

	return buf.Bytes()

}

func (site *Site) readResponse(response *http.Response) ([]byte, error) {
	contentEncoding := response.Header.Get("Content-Encoding")
	var content []byte
	var err error
	if contentEncoding == "gzip" {
		reader, gzipErr := gzip.NewReader(response.Body)
		if gzipErr != nil {
			return content, gzipErr
		}
		content, err = ioutil.ReadAll(reader)
	} else {
		content, err = ioutil.ReadAll(response.Body)
	}
	return content, err
}

func (site *Site) EncodeUrl(u *url.URL) {
	if isIndexPage(u) {
		return
	}
	requestPath, file := filepath.Split(u.Path)
	pathRunes := []rune(requestPath)
	for key, v := range pathRunes {
		if v >= 65 && v < 90 {
			pathRunes[key] = v + 1
		} else if v == 90 {
			pathRunes[key] = 65
		} else if v >= 97 && v < 122 {
			pathRunes[key] = v + 1
		} else if v == 122 {
			pathRunes[key] = 97
		}
	}
	has := md5.Sum([]byte(requestPath))
	md5str := fmt.Sprintf("%x", has)
	md5path := md5str[:5] + "_"
	u.Path = string(pathRunes[:1]) + md5path + string(pathRunes[1:]) + file
}
func (site *Site) DecodeUrl(u *url.URL) {
	if isIndexPage(u) {
		return
	}

	r, _ := regexp.Compile(`^/([a-f0-9]{5}_)`)
	matches := r.FindStringSubmatch(u.Path)
	if len(matches) != 2 {
		return
	}
	p := strings.Replace(u.Path, matches[1], "", 1)
	//p := r.ReplaceAllString(u.Path, "")
	requestPath, file := filepath.Split(p)
	pathRunes := []rune(requestPath)
	for key, value := range pathRunes {
		if value > 65 && value <= 90 {
			pathRunes[key] = value - 1
		} else if value == 65 {
			pathRunes[key] = 90
		} else if value > 97 && value <= 122 {
			pathRunes[key] = value - 1
		} else if value == 97 {
			pathRunes[key] = 122
		}
	}
	u.Path = string(pathRunes) + file

}

func (site *Site) replaceHost(content string, host string) string {
	u, _ := url.Parse(site.Url)
	content = strings.Replace(content, u.Host, host, -1)
	if site.Schema == "https" {
		content = strings.Replace(content, "http://"+host, "https://"+host, -1)
	} else {
		content = strings.Replace(content, "https://"+host, "http://"+host, -1)
	}
	hostParts := strings.Split(u.Host, ".")
	originHost := strings.Join(hostParts[1:], ".")
	subDomainRegexp, _ := regexp.Compile(`[a-zA-Z0-9]+\.` + originHost)
	content = subDomainRegexp.ReplaceAllString(content, "")
	content = strings.Replace(content, originHost, site.Domain, -1)
	return content
}
func (site *Site) transformTitleNode(node *html.Node, isIndexPage bool) {
	if isIndexPage {
		if node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
			node.FirstChild.Data = "{{index_title}}"
			return
		}
		node.FirstChild = &html.Node{
			Type: html.TextNode,
			Data: "{{index_title}}",
		}
		return
	}

	if !isIndexPage && len(site.app.Keywords) > 0 && node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
		title := node.FirstChild.Data
		randIndex := rand.Intn(len(site.app.Keywords))
		d := []rune(title)
		length := strings.Count(title, "")
		n := rand.Intn(length)
		tag := fmt.Sprintf("{{keyword:%d}}", randIndex)
		title = string(d[:n]) + tag + string(d[n:])
		node.FirstChild.Data = title
	}
}

func (site *Site) setCache(url string, response *http.Response, content []byte, host string) error {
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "charset") {
		contentType := response.Header.Get("Content-Type")
		contentPartArr := strings.Split(contentType, ";")
		response.Header.Set("Content-Type", contentPartArr[0]+"; charset=utf-8")
	}
	response.Header.Del("Content-Encoding")
	resp := &CustomResponse{
		StatusCode: response.StatusCode,
		Body:       content,
		Header:     response.Header,
	}

	sum := sha1.Sum([]byte(url))
	hash := hex.EncodeToString(sum[:])
	dir := path.Join(site.CachePath, host, hash[:5])
	if !isExist(dir) {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			site.app.Logger.Error("MkdirAll error", dir, err.Error())
			return err
		}
	}
	filename := path.Join(dir, hash)
	file, err := os.Create(filename)
	if err != nil {
		site.app.Logger.Error("os.Create error", filename, err.Error())
		return err
	}
	defer file.Close()
	if err := gob.NewEncoder(file).Encode(resp); err != nil {
		site.app.Logger.Error("gob.NewEncoder error", filename, err.Error())
		return err
	}
	return nil
}
func (site *Site) getCache(requestUrl string, host string, force bool) *CustomResponse {
	sum := sha1.Sum([]byte(requestUrl))
	hash := hex.EncodeToString(sum[:])
	dir := path.Join(site.CachePath, host, hash[:5])
	filename := path.Join(dir, hash)
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return nil
	}
	if modTime := fileInfo.ModTime(); !force && time.Now().Unix() > modTime.Unix()+site.CacheTime*60 {
		return nil
	}

	if file, err := os.Open(filename); err == nil {
		resp := new(CustomResponse)
		gob.NewDecoder(file).Decode(resp)
		file.Close()
		return resp
	}
	return nil
}

func isExist(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		return os.IsExist(err)
	}
	return true

}
func (site *Site) injectJs(content []byte, isIndexPage bool, isSpider bool) []byte {
	if !isSpider {
		titleRegexp, _ := regexp.Compile(`(?i)</title>`)
		content = titleRegexp.ReplaceAll(content, []byte("</title>\n<script type=\"text/javascript\" src=\""+site.app.InjectJsPath+"\"></script>"))
	}
	if friendLink := site.friendLink(site.Domain); isIndexPage && friendLink != "" {
		bodyRegexp, _ := regexp.Compile(`(?i)</body>`)
		content = bodyRegexp.ReplaceAll(content, []byte(friendLink+"</body>"))
	}
	return content
}
func (site Site) friendLink(domain string) string {
	if len(site.app.FriendLinks[domain]) <= 0 {
		return ""
	}
	var friendLink string
	for _, link := range site.app.FriendLinks[domain] {
		linkItem := strings.Split(link, ",")
		if len(linkItem) != 2 {
			continue
		}
		friendLink += fmt.Sprintf("<a href='%s' target='_blank'>%s</a>", linkItem[0], linkItem[1])
	}
	return fmt.Sprintf("<div style='display:none'>%s</div>", friendLink)
}
func (site *Site) isCrawler(ua string) bool {

	ua = strings.ToLower(ua)
	for _, value := range site.app.Spider {
		spider := strings.ToLower(value)
		if strings.Contains(ua, spider) {
			return true
		}
	}
	return false
}
func (site *Site) isGoodCrawler(ua string) bool {
	ua = strings.ToLower(ua)
	for _, value := range site.app.GoodSpider {
		spider := strings.ToLower(value)
		if strings.Contains(ua, spider) {
			return true
		}
	}
	return false
}
func (site *Site) wrapResponseBody(response *http.Response, content []byte) {
	readAndCloser := ioutil.NopCloser(bytes.NewReader(content))
	contentLength := int64(len(content))
	response.Body = readAndCloser
	response.ContentLength = contentLength
	response.Header.Set("Content-Length", strconv.FormatInt(contentLength, 10))
}
func (site *Site) ErrorHandler(writer http.ResponseWriter, request *http.Request, e error) {
	site.app.Logger.Error(request.URL.String(), e.Error())
	host := request.Context().Value("HOST").(string)
	cacheKey := host + request.URL.Path + request.URL.RawQuery
	cacheResponse := site.getCache(cacheKey, host, true)
	if cacheResponse == nil {
		writer.WriteHeader(404)
		writer.Write([]byte("请求出错，请检查源站"))
		return

	}
	for s, i := range cacheResponse.Header {
		writer.Header()[s] = i
	}
	var content = cacheResponse.Body
	if strings.Contains(strings.ToLower(cacheResponse.Header.Get("Content-Type")), "html") {
		ua := request.Context().Value("Origin-Ua").(string)
		content = site.injectJs(content, isIndexPage(request.URL), site.isCrawler(ua))
	}
	contentLength := int64(len(content))
	writer.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	if cacheResponse.StatusCode != 0 {
		writer.WriteHeader(cacheResponse.StatusCode)
	} else {
		writer.WriteHeader(200)
	}
	_, err := writer.Write(content)
	if err != nil {
		site.app.Logger.Error("写出错误：", err.Error(), request.URL)
	}

}
