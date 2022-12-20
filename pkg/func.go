package pkg

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html/charset"
)

func GetHost(request *http.Request) string {
	host := request.Host
	if host == "" {
		host = request.Header.Get("Host")
	}
	if strings.Index(host, ":") != -1 {
		host, _, _ = net.SplitHostPort(host)
	}
	return host
}
func singleJoiningSlash(a, b string) string {
	asLash := strings.HasSuffix(a, "/")
	bsLash := strings.HasPrefix(b, "/")
	switch {
	case asLash && bsLash:
		return a + b[1:]
	case !asLash && !bsLash:
		return a + "/" + b
	}
	return a + b
}
func isIndexPage(u *url.URL) bool {
	return (u.Path == "" ||
		u.Path == "/" ||
		strings.ToLower(u.Path) == "/index.php" ||
		strings.ToLower(u.Path) == "/index.asp" ||
		strings.ToLower(u.Path) == "/index.jsp" ||
		strings.ToLower(u.Path) == "/index.htm" ||
		strings.ToLower(u.Path) == "/index.html" ||
		strings.ToLower(u.Path) == "/index.shtml")

}
func GBk2UTF8(content []byte, contentType string) string {
	e, name, _ := charset.DetermineEncoding(content, contentType)
	if strings.ToLower(name) != "utf-8" {
		content, _ = e.NewDecoder().Bytes(content)
	}
	var contentStr = string(content)
	gbkArr := []string{"gb2312", "gbk", "GBK", "GB2312"}
	for _, g := range gbkArr {
		contentStr = strings.Replace(contentStr, g, "utf-8", -1)
	}
	return contentStr
}
