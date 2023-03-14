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
	Scheme    string
	app       *App
	CachePath string
}
type CustomResponse struct {
	StatusCode int
	// Body is the content of the Response
	Body []byte
	// Headers contains the Response's HTTP headers
	Header http.Header

	RandomHtml string
}
type Key uint

const (
	ORIGIN_UA Key = iota
	REQUEST_HOST
)

// var BufferPool *sync.Pool = &sync.Pool{
// 	New: func() any {
// 		return bytes.NewBuffer(make([]byte, 0))
// 	},
// }
// var CustomResponsePool *sync.Pool = &sync.Pool{
// 	New: func() any {
// 		return new(CustomResponse)
// 	},
// }

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
	requestHost := GetHost(request)
	request = request.WithContext(context.WithValue(context.WithValue(request.Context(), ORIGIN_UA, ua), REQUEST_HOST, requestHost))

	if site.isCrawler(ua) && !site.isGoodCrawler(ua) { //如果是蜘蛛但不是好蜘蛛
		writer.WriteHeader(404)
		_, _ = writer.Write([]byte("页面未找到"))
		return
	}

	cacheKey := site.Domain + request.URL.Path + request.URL.RawQuery
	if site.CacheEnable {
		if cacheResponse := site.getCache(cacheKey, false); cacheResponse != nil {
			//defer CustomResponsePool.Put(cacheResponse)
			contentType := strings.ToLower(cacheResponse.Header.Get("Content-Type"))
			var content []byte = cacheResponse.Body
			if strings.Contains(contentType, "text/html") {
				isIndexPage := isIndexPage(request.URL)
				isSpider := site.isCrawler(ua)
				content = site.handleHtmlResponse(content, isIndexPage, isSpider, contentType, requestHost, cacheResponse.RandomHtml)

				if isSpider && cacheResponse.StatusCode == 200 {
					site.app.AddRecord(requestHost, request.URL.Path, ua)
				}
			} else if strings.Contains(contentType, "css") || strings.Contains(contentType, "javascript") {
				content = GBk2UTF8(content, contentType)
				contentStr := site.replaceHost(string(content), requestHost)
				content = []byte(contentStr)
			}

			for key, values := range cacheResponse.Header {
				writer.Header()[key] = values
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
				site.app.Logger.Error("写出错误：", err.Error(), requestHost, request.URL)
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
	requestHost := response.Request.Context().Value(REQUEST_HOST).(string)
	if response.StatusCode == 301 || response.StatusCode == 302 {
		return site.handleRedirectResponse(response, requestHost)
	}

	cacheKey := site.Domain + response.Request.URL.Path + response.Request.URL.RawQuery
	if response.StatusCode == 200 {
		content, err := site.readResponse(response)
		if err != nil {
			return err
		}
		contentType := strings.ToLower(response.Header.Get("Content-Type"))
		if strings.Contains(contentType, "text/html") {
			content = bytes.ReplaceAll(content, []byte("\u200B"), []byte(""))
			content = bytes.ReplaceAll(content, []byte("\uFEFF"), []byte(""))
			content = bytes.ReplaceAll(content, []byte("\u200D"), []byte(""))
			content = bytes.ReplaceAll(content, []byte("\u200C"), []byte(""))
			randomHtml := RandHtml(site.Domain, site.Scheme)
			_ = site.setCache(cacheKey, response.StatusCode, response.Header, content, randomHtml)
			originUa := response.Request.Context().Value(ORIGIN_UA).(string)
			isSpider := site.isCrawler(originUa)
			content = site.handleHtmlResponse(content, isIndexPage(response.Request.URL), isSpider, contentType, requestHost, randomHtml)
			site.wrapResponseBody(response, content)
			if isSpider {
				site.app.AddRecord(requestHost, response.Request.URL.Path, originUa)
			}
			return nil
		} else if strings.Contains(contentType, "css") || strings.Contains(contentType, "javascript") {
			_ = site.setCache(cacheKey, response.StatusCode, response.Header, content, "")
			content = GBk2UTF8(content, contentType)
			contentStr := site.replaceHost(string(content), requestHost)
			content = []byte(contentStr)
			site.wrapResponseBody(response, content)
			return nil

		}
		_ = site.setCache(cacheKey, response.StatusCode, response.Header, content, "")
		site.wrapResponseBody(response, content)
		return nil

	}
	if response.StatusCode > 400 && response.StatusCode < 500 {
		content := []byte("访问的页面不存在")
		_ = site.setCache(cacheKey, response.StatusCode, response.Header, content, "")
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
	redirectUrl.Scheme = site.Scheme
	response.Header.Set("Location", redirectUrl.String())
	return nil
}
func (site *Site) handleHtmlNode(node *html.Node, requestHost string, isIndexPage bool, replacedH1 *bool) {
	switch node.Type {
	case html.TextNode, html.CommentNode, html.RawNode:
		node.Data = site.transformText(node.Data)
	case html.ElementNode:
		if node.Data == "a" {
			site.transformANode(node, requestHost)
		}
		if node.Data == "link" {
			site.transformLinkNode(node, requestHost)
		}
		if node.Data == "title" {
			site.transformTitleNode(node, isIndexPage)
		}
		if node.Data == "script" {
			site.transformScriptNode(node)
		}
		if node.Data == "meta" {
			site.transformMetaNode(node, isIndexPage)
		}
		if node.Data == "body" {
			node.InsertBefore(&html.Node{
				Type: html.TextNode,
				Data: "{{random_html}}",
			}, node.FirstChild)
			if isIndexPage {
				node.AppendChild(&html.Node{
					Type: html.TextNode,
					Data: "{{friend_links}}",
				})
			}
		}
		if node.Data == "head" {
			node.AppendChild(&html.Node{
				Type: html.TextNode,
				Data: "{{inject_js}}",
			})
		}
		if node.Data == "h1" && node.FirstChild != nil && node.FirstChild.Type == html.TextNode && site.H1Replace != "" {
			node.FirstChild.Data = site.H1Replace
			*replacedH1 = true
		}
		for i, attr := range node.Attr {
			// if attr.Key == "href" || attr.Key == "src" {
			// 	node.Attr[i].Val = site.replaceHost(attr.Val, requestHost)
			// }
			if attr.Key == "title" || attr.Key == "alt" || attr.Key == "value" || attr.Key == "placeholder" {
				for index, find := range site.Finds {
					tag := fmt.Sprintf("{{replace:%d}}", index)
					attr.Val = strings.ReplaceAll(attr.Val, find, tag)
				}
				node.Attr[i].Val = attr.Val
				if site.S2t {
					node.Attr[i].Val, _ = site.app.S2T.Convert(attr.Val)
				}
			}
		}

	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		site.handleHtmlNode(c, requestHost, isIndexPage, replacedH1)
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
func (site *Site) transformText(text string) string {
	for index, find := range site.Finds {
		tag := fmt.Sprintf("{{replace:%d}}", index)
		text = strings.ReplaceAll(text, find, tag)
	}
	//text = site.replaceHost(text, requestHost)
	if site.S2t {
		chineseRegexp, _ := regexp.Compile("^[\u4e00-\u9fa5]+")
		text = chineseRegexp.ReplaceAllStringFunc(text, func(s string) string {
			result, _ := site.app.S2T.Convert(s)
			return result
		})
	}
	return text
}
func (site *Site) transformLinkNode(node *html.Node, requestHost string) {
	isAlternate := false
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
			node.Attr[i].Val = "//" + requestHost
			break
		}
	}
}
func (site *Site) transformANode(node *html.Node, requestHost string) {
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
			u.Scheme = site.Scheme
			u.Host = requestHost
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
func (site *Site) parseTemplateTags(content []byte, requestHost string, randomHtml string, isIndexPage bool) []byte {
	contentStr := string(content)
	contentStr = site.replaceHost(contentStr, requestHost)
	contentStr = strings.Replace(contentStr, "{{index_title}}", site.IndexTitle, 1)
	contentStr = strings.Replace(contentStr, "{{index_keywords}}", site.IndexKeywords, 1)
	contentStr = strings.Replace(contentStr, "{{index_description}}", site.IndexDescription, 1)
	contentStr = strings.Replace(contentStr, "{{random_html}}", randomHtml, 1)
	injectJs := fmt.Sprintf(`<script type="text/javascript" src="%s"></script>`, site.app.InjectJsPath)
	contentStr = strings.Replace(contentStr, "{{inject_js}}", injectJs, 1)
	if isIndexPage {
		friendLink := site.friendLink(site.Domain)
		contentStr = strings.Replace(contentStr, "{{friend_links}}", friendLink, 1)
	}

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
func (site *Site) handleHtmlResponse(content []byte, isIndexPage bool, isSpider bool, contentType string, requestHost string, randomHtml string) []byte {
	content = GBk2UTF8(content, contentType)
	content = site.handleHtmlContent(content, requestHost, isIndexPage)
	content = site.parseTemplateTags(content, requestHost, randomHtml, isIndexPage)
	return content

}
func (site *Site) handleHtmlContent(content []byte, requestHost string, isIndexPage bool) []byte {
	document, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		site.app.Logger.Error("html parse error", err.Error())
		return content
	}
	replacedH1 := false
	for c := document.FirstChild; c != nil; c = c.NextSibling {
		site.handleHtmlNode(c, requestHost, isIndexPage, &replacedH1)
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
	if contentEncoding == "gzip" {
		reader, gzipErr := gzip.NewReader(response.Body)
		if gzipErr != nil {
			return nil, gzipErr
		}
		content, err := ioutil.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		return content, nil
	}
	content, err := ioutil.ReadAll(response.Body)
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

	r, _ := regexp.Compile(`^/([a-f\d]{5}_)`)
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

func (site *Site) replaceHost(content string, requestHost string) string {
	u, _ := url.Parse(site.Url)
	content = strings.ReplaceAll(content, u.Host, requestHost)
	if site.Scheme == "https" {
		content = strings.ReplaceAll(content, "http://"+requestHost, "https://"+requestHost)
	} else {
		content = strings.ReplaceAll(content, "https://"+requestHost, "http://"+requestHost)
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

func (site *Site) setCache(url string, statusCode int, header http.Header, content []byte, randomHtml string) error {
	contentType := header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "charset") {
		contentPartArr := strings.Split(contentType, ";")
		header.Set("Content-Type", contentPartArr[0]+"; charset=utf-8")
	}
	header.Del("Content-Encoding")
	resp := &CustomResponse{
		Body:       content,
		StatusCode: statusCode,
		Header:     header,
		RandomHtml: randomHtml,
	}
	sum := sha1.Sum([]byte(url))
	hash := hex.EncodeToString(sum[:])
	dir := path.Join(site.CachePath, site.Domain, hash[:5])
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
func (site *Site) getCache(requestUrl string, force bool) *CustomResponse {
	sum := sha1.Sum([]byte(requestUrl))
	hash := hex.EncodeToString(sum[:])
	dir := path.Join(site.CachePath, site.Domain, hash[:5])
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
	requestHost := request.Context().Value(REQUEST_HOST).(string)
	ua := request.Context().Value(ORIGIN_UA).(string)
	cacheKey := site.Domain + request.URL.Path + request.URL.RawQuery
	cacheResponse := site.getCache(cacheKey, true)
	if cacheResponse == nil {
		writer.WriteHeader(404)
		writer.Write([]byte("请求出错，请检查源站"))
		return

	}

	var content = cacheResponse.Body
	contentType := strings.ToLower(cacheResponse.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") {
		isIndexPage := isIndexPage(request.URL)
		isSpider := site.isCrawler(ua)
		content = site.handleHtmlResponse(content, isIndexPage, isSpider, contentType, requestHost, cacheResponse.RandomHtml)
		if isSpider && cacheResponse.StatusCode == 200 {
			site.app.AddRecord(requestHost, request.URL.Path, ua)
		}
	} else if strings.Contains(contentType, "css") || strings.Contains(contentType, "javascript") {
		content = GBk2UTF8(content, contentType)
		contentStr := site.replaceHost(string(content), requestHost)
		content = []byte(contentStr)
	}

	for s, i := range cacheResponse.Header {
		writer.Header()[s] = i
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
