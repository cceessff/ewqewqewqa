package pkg

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/wenzhenxi/gorsa"
	"golang.org/x/net/html/charset"
)

func GetHost(request *http.Request) string {
	host := request.Host
	if host == "" {
		host = request.Header.Get("Host")
	}
	if strings.Contains(host, ":") {
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

func newProxy(target *url.URL, ipList []net.IP) *httputil.ReverseProxy {
	targetQuery := target.RawQuery
	director := func(req *http.Request) {
		req.Host = target.Host
		req.Header.Set("Referer", target.Scheme+"://"+target.Host)
		req.Header.Del("If-Modified-Since")
		req.Header.Del("If-None-Match")
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to default value
			req.Header.Set("User-Agent", "")
		}
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var localIp net.IP = net.IPv4(0, 0, 0, 0)
			if len(ipList) > 0 {
				ipIndex := rand.Intn(len(ipList))
				localIp = ipList[ipIndex]
			}
			localAddr := &net.TCPAddr{IP: localIp, Port: 0, Zone: ""}
			var dialer = net.Dialer{
				LocalAddr: localAddr,
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	return &httputil.ReverseProxy{Director: director, Transport: transport}
}
func GetIPList() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Println("获取本地IP错误" + err.Error())
		return nil
	}
	ipList := make([]net.IP, 0)
	for _, address := range addrs {
		// 检查ip地址判断是否回环地址
		ipNet, ok := address.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil && isPublicIP(ipNet.IP) {
			ipList = append(ipList, ipNet.IP)
		}
	}
	return ipList
}

func isPublicIP(IP net.IP) bool {
	if IP.IsLoopback() || IP.IsLinkLocalMulticast() || IP.IsLinkLocalUnicast() {
		return false
	}
	if ip4 := IP.To4(); ip4 != nil {
		switch true {
		case ip4[0] == 10:
			return false
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return false
		case ip4[0] == 192 && ip4[1] == 168:
			return false
		default:
			return true
		}
	}
	return false
}
func RandHtml() string {
	htmlTags := []string{"abbr", "address", "area", "article", "aside", "b", "base", "bdo", "blockquote", "button", "cite", "code", "dd", "del", "details", "dfn", "dl", "dt", "em", "figure", "font", "i", "ins", "kbd", "label", "legend", "li", "mark", "meter", "ol", "option", "p", "q", "progress", "rt", "ruby", "samp", "section", "select", "small", "strong", "tt", "u"}
	var result string
	for i := 0; i < 100; i++ {
		t := htmlTags[rand.Intn(len(htmlTags))]
		result = result + fmt.Sprintf(`<%s id="%s">%s</%s>`, t, RandStr(2), RandStr(10), t)
	}
	return "<div style=\"display:none\">" + result + "</div>"
}
func RandStr(count int) string {
	chars := []rune("ABCDEFGHIJKLNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	count = rand.Intn(count) + 6
	result := ""
	for i := 0; i < count; i++ {
		result = result + string(chars[rand.Intn(len(chars))])
	}
	return result

}
func readLinks() map[string][]string {
	result := make(map[string][]string)
	linkData, err := ioutil.ReadFile("config/links.txt")
	if err != nil && len(linkData) <= 0 {
		return result
	}
	linkLines := strings.Split(strings.Replace(string(linkData), "\r", "", -1), "\n")
	for _, line := range linkLines {
		linkArr := strings.Split(line, "||")
		if len(linkArr) < 2 {
			continue
		}
		result[linkArr[0]] = linkArr[1:]
	}
	return result
}

func ParseAppConfig() (AppConfig, error) {
	var appConfig AppConfig
	data, err := ioutil.ReadFile("config/config.json")
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

func GetExpireDate() (string, error) {
	pubKye := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAsfUtexjm9RVM5CpijrNF
NDI4NfCyMIxW9q+/QaBXiNbqoguWYh1Mmkt+tal6QqObyvmufAbMfJpj0b+cGm96
KYgAOXUntYAKkTvQLQoQQl9aGY/rxEPuVu+nvN0zsVHrDteaWpMu+7O6OyYS0aKL
nWhCYpobTp6MTheMfnlMi7p2pJmGxyvUvZNvv6O6OZelOyr7Pb1FeYzpc/8+vkmK
BGnbyK6EVbZ5vwTaw/X2DI4uDOneKU2qVUyq2nd7pSvbX9aSuQZq1xwWhIXcEY6l
XzFBxZbhjXaZkaO2CWTHLwcKtSCCd3PkXNCRWQeHM4OelRZJajKSxwcWWTqbusGC
2wIDAQAB
-----END PUBLIC KEY-----`

	certBytes, err := ioutil.ReadFile("config/auth.cert")
	if err != nil {
		return "", err
	}
	hexData, err := gorsa.PublicDecrypt(string(certBytes), pubKye)
	if err != nil {
		return "", errors.New("认证失败，无法解密," + err.Error())
	}
	data, err := hex.DecodeString(hexData)
	if err != nil {
		return "", errors.New("认证失败，无法解码," + err.Error())
	}
	return string(data), nil
}
