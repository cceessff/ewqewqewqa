package pkg

import (
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type Site struct {
	*SiteConfig
	*httputil.ReverseProxy
	Schema string
	app    *App
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
			return site.handleHtmlResponse(response)

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
func (site *Site) handleHtmlContent(content []byte, contentType string) error {
	var contentStr = GBk2UTF8(content, contentType)
	document, err := html.Parse(strings.NewReader(contentStr))
	if err != nil {
		return err
	}

	for _, item := range site.app.GlobalReplace {
		contentStr = strings.Replace(contentStr, item["needle"], item["replace"], -1)
	}
	if site.S2t {
		chineseWordRegexp := regexp.MustCompile("[\u4e00-\u9fa5]+")
		contentStr = chineseWordRegexp.ReplaceAllStringFunc(contentStr, func(s string) string {
			s, _ = site.siteManager.S2T.ConvertText(s)
			return s
		})
		var tfind = make([]string, 0)
		for _, find := range siteConf.Finds {
			t, _ := site.siteManager.S2T.ConvertText(find)
			tfind = append(tfind, t)
		}
		contentStr = site.ReplaceWords(contentStr, tfind, siteConf.Replaces, contentType)

	} else {
		contentStr = site.ReplaceWords(contentStr, siteConf.Finds, siteConf.Replaces, contentType)

	}
	contentStr = site.Link(contentStr, siteConf, response.Request.URL)
	contentStr = site.replaceHost(contentStr)
	if len(site.siteManager.Keywords) > 0 {
		keywords := site.siteManager.Keywords
		contentStr = site.changeTitle(contentStr, keywords[rand.Intn(len(keywords))], siteConf.TitleReplace)
	}
	if site.isIndexPage(response.Request.URL) {
		contentStr = site.IndexPage(contentStr, siteConf)
	}
	if !siteConf.NeedJs {
		regx, _ := regexp.Compile(`(?i)<script[\S\s]+?</script>`)
		contentStr = regx.ReplaceAllString(contentStr, "")
	}

	if siteConf.H1Replace != "" {
		contentStr = site.H1replace(contentStr, siteConf.H1Replace)
	}
	contentStr = strings.Replace(contentStr, "<body>", "<body>"+site.RandHtml(), 1)
	myResponse := site.ToResponse(response, []byte(contentStr))
	site.setCache(cacheKey, myResponse)
	content = site.injectJs([]byte(contentStr), site.isIndexPage(response.Request.URL), site.isCrawler(response.Request.Header.Get("Origin-Ua")))
	site.changeResponseBody(response, content)
	return nil
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
