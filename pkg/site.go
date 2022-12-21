package pkg

import (
	"bytes"
	"compress/gzip"
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

func (site *Site) ModifyResponse(response *http.Response) error {

	if response.StatusCode == 301 || response.StatusCode == 302 {
		return site.handleRedirectResponse(response)
	}
	cacheKey := site.Domain + response.Request.URL.Path + response.Request.URL.RawQuery
	if response.StatusCode == 200 {
		content, err := site.readResponse(response)
		if err != nil {
			return err
		}
		contentType := strings.ToLower(response.Header.Get("Content-Type"))

		if strings.Index(contentType, "text/html") != -1 {
			return site.handleHtmlResponse(content, response, contentType)

		} else if strings.Index(contentType, "css") != -1 || strings.Index(contentType, "javascript") != -1 {
			var contentStr = site.GBk2UTF8(content, contentType)
			u, _ := url.Parse(siteConf.Url)
			contentStr = strings.Replace(contentStr, u.Host, siteConf.Domain, -1)
			response.Request.URL.Host = siteConf.Domain

			if site.schema == "https" {
				contentStr = strings.Replace(contentStr, "http:", "https:", -1)
			} else {
				contentStr = strings.Replace(contentStr, "https:", "http:", -1)
			}
			myResponse := site.ToResponse(response, []byte(contentStr))
			site.setCache(cacheKey, myResponse)
			site.changeResponseBody(response, []byte(contentStr))
			return nil

		} else {
			myResponse := site.ToResponse(response, content)
			site.setCache(cacheKey, myResponse)
			site.changeResponseBody(response, content)
			return nil
		}
	}

	if response.StatusCode > 400 && response.StatusCode < 500 {
		content := []byte("访问的页面不存在")
		myResponse := site.ToResponse(response, content)
		site.setCache(cacheKey, myResponse)
		site.changeResponseBody(response, content)
	}
	return nil
}

func (site *Site) handleRedirectResponse(response *http.Response) error {
	redirectUrl, err := response.Request.URL.Parse(response.Header.Get("Location"))
	if err != nil {
		return err
	}
	redirectUrl.Host = site.Domain
	redirectUrl.Scheme = site.Schema
	if !isIndexPage(redirectUrl) {
		site.EncodeUrl(redirectUrl)
	}
	response.Header.Set("Location", redirectUrl.String())
	return nil
}
func (site *Site) handleHtmlNode(node *html.Node, isIndexPage bool) {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode, html.CommentNode, html.RawNode:
			c.Data = site.transformText(c.Data)
		case html.ElementNode:
			if c.Data == "a" {
				site.transformANode(c)
			}
			if c.Data == "link" {
				site.transformLinkNode(c)
			}
			if c.Data == "title" {
				site.transformTitleNode(c, isIndexPage)
			}
			for _, attr := range c.Attr {
				if attr.Key == "href" || attr.Key == "src" {
					attr.Val = site.replaceHost(attr.Val)
				}
			}

		}
		site.handleHtmlNode(c, isIndexPage)
	}
}
func (site *Site) transformText(text string) string {
	for _, item := range site.app.GlobalReplace {
		text = strings.Replace(text, item["needle"], item["replace"], -1)
	}
	for index, find := range site.Finds {
		replace := site.Replaces[index]
		text = strings.Replace(text, find, site.htmlEntities(replace), -1)
	}
	text = site.replaceHost(text)
	if site.S2t {
		text, _ = site.app.S2T.ConvertText(text)
	}
	return text
}
func (site *Site) transformLinkNode(node *html.Node) {
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
	for _, attr := range node.Attr {
		if attr.Key == "href" {
			attr.Val = "//" + site.Domain
			break
		}
	}
}
func (site *Site) transformANode(node *html.Node) {
	ou, _ := url.Parse(site.Url)
	for _, attr := range node.Attr {
		if attr.Key != "href" || attr.Val == "" {
			continue
		}
		u, _ := ou.Parse(attr.Val)
		if u == nil {
			continue
		}
		if u.Host == ou.Host {
			u.Scheme = site.Schema
			u.Host = site.Domain
			attr.Val = u.String()
			return
		}
		if u.Path == "" {
			//path为空，是友情链接，全部删除
			node = nil
		} else {
			//不是友情链接，只删除链接，不删除文字
			attr.Val = "#"
		}
	}
}
func (site *Site) handleHtmlResponse(content []byte, response *http.Response, contentType string) error {
	isIndexPage := isIndexPage(response.Request.URL)
	contentStr := site.handleHtmlContent(content, contentType, isIndexPage)
	cacheKey := site.Domain + response.Request.URL.Path + response.Request.URL.RawQuery
	site.setCache(cacheKey, response, []byte(contentStr))
	content = site.injectJs([]byte(contentStr), isIndexPage, site.isCrawler(response.Request.Header.Get("Origin-Ua")))
	site.changeResponseBody(response, content)

}
func (site *Site) handleHtmlContent(content []byte, contentType string, isIndexPage bool) string {
	var contentStr = GBk2UTF8(content, contentType)
	document, err := html.Parse(strings.NewReader(contentStr))
	if err != nil {
		return contentStr
	}

	for c := document.FirstChild; c != nil; c = c.NextSibling {
		site.handleHtmlNode(c, isIndexPage)
	}
	var buf bytes.Buffer
	err = html.Render(&buf, document)
	if err != nil {
		return contentStr
	}
	return buf.String()

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
func (site *Site) htmlEntities(input string) string {
	runes := []rune(input)
	var buffer bytes.Buffer

	for _, r := range runes {
		inputUnicode := strconv.QuoteToASCII(string(r))
		if strings.Contains(inputUnicode, "\\u") {
			inputUnicode = strings.Replace(inputUnicode, `"`, "", 2)
			inputUnicode = strings.Replace(inputUnicode, "\\u", "", 1)
			code, _ := strconv.ParseUint(inputUnicode, 16, 64)
			entity := fmt.Sprintf("&#%d;", code)
			buffer.WriteString(entity)

		} else {
			buffer.WriteString(string(r))
		}
	}
	return buffer.String()
}
func (site *Site) replaceHost(content string) string {
	u, _ := url.Parse(site.Url)
	content = strings.Replace(content, u.Host, site.Domain, -1)
	if site.Schema == "https" {
		content = strings.Replace(content, "http://"+site.Domain, "https://"+site.Domain, -1)
	} else {
		content = strings.Replace(content, "https://"+site.Domain, "http://"+site.Domain, -1)
	}
	hostParts := strings.Split(u.Host, ".")
	host := strings.Join(hostParts[1:], ".")
	subDomainRegexp, _ := regexp.Compile(`[a-zA-Z0-9]+\.` + host)
	content = subDomainRegexp.ReplaceAllString(content, "")
	content = strings.Replace(content, host, site.Domain, -1)
	return content
}
func (site *Site) transformTitleNode(node *html.Node, isIndexPage bool) {
	if isIndexPage {
		title := site.IndexTitle
		if node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
			node.FirstChild.Data = title
			return
		}
		node.FirstChild = &html.Node{
			Type: html.TextNode,
			Data: title,
		}
		return
	}

	if !isIndexPage && len(site.app.Keywords) > 0 && node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
		title := node.FirstChild.Data
		randIndex := rand.Intn(len(site.app.Keywords))
		keywrod := site.app.Keywords[randIndex]
		d := []rune(title)
		length := strings.Count(title, "")
		n := rand.Intn(length)
		title = string(d[:n]) + keywrod + string(d[n:])
		node.FirstChild.Data = title

	}
}

func (site *Site) setCache(url string, response *http.Response, content []byte) error {
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
	dir := path.Join(site.CachePath, site.Domain, hash[:5])
	if !isExist(dir) {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	}
	filename := path.Join(dir, hash)
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := gob.NewEncoder(file).Encode(resp); err != nil {
		return err
	}
	return nil
}
func isExist(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true

}
func (site *Site) injectJs(content []byte, isIndexPage bool, isSpider bool) []byte {
	contentStr := string(content)
	explanatoryRegexp, _ := regexp.Compile(`<!--[\s\S]*?-->`)
	contentStr = explanatoryRegexp.ReplaceAllString(contentStr, "")
	if !isSpider {
		titleRegexp, _ := regexp.Compile(`(?i)</title>`)
		jsSrc := "/abcd/abcd.js"
		contentStr = titleRegexp.ReplaceAllLiteralString(contentStr, "</title>\n<script type=\"text/javascript\" src=\""+jsSrc+"\"></script>")
	}
	if friendLink := site.friendLink(site.Domain); isIndexPage && friendLink != "" {
		bodyRegexp, _ := regexp.Compile(`(?i)</body>`)
		contentStr = bodyRegexp.ReplaceAllString(contentStr, friendLink+"</body>")
	}
	return []byte(contentStr)
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
		if strings.Contains(ua, spider) != false {
			return true
		}
	}
	return false
}
func (site *Site) isGoodCrawler(ua string) bool {
	ua = strings.ToLower(ua)
	for _, value := range site.app.GoodSpider {
		spider := strings.ToLower(value)
		if strings.Contains(ua, spider) != false {
			return true
		}
	}
	return false
}
