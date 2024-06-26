package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	return u.Path == "" ||
		strings.EqualFold(u.Path, "/") ||
		strings.EqualFold(u.Path, "/index.php") ||
		strings.EqualFold(u.Path, "/index.asp") ||
		strings.EqualFold(u.Path, "/index.jsp") ||
		strings.EqualFold(u.Path, "/index.htm") ||
		strings.EqualFold(u.Path, "/index.html") ||
		strings.EqualFold(u.Path, "/index.shtml")

}
func GBk2UTF8(content []byte, contentType string) []byte {
	temp := content
	if len(content) > 1024 {
		temp = content[:1024]
	}
	if !IsUTF8(temp) {
		e, name, _ := charset.DetermineEncoding(content, contentType)
		if !strings.EqualFold(name, "utf-8") {
			content, _ = e.NewDecoder().Bytes(content)
		}
	}
	return content
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
func GetIPList() ([]net.IP, error) {
	ipList := make([]net.IP, 0)
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return ipList, err
	}
	for _, address := range addresses {
		// 检查ip地址判断是否回环地址
		ipNet, ok := address.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil && isPublicIP(ipNet.IP) {
			ipList = append(ipList, ipNet.IP)
		}
	}
	return ipList, nil
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
func RandHtml(domain string, schema string) string {
	htmlTags := []string{"abbr", "address", "area", "article", "aside", "b", "base", "bdo", "blockquote", "button", "cite", "code", "dd", "del", "details", "dfn", "dl", "dt", "em", "figure", "font", "i", "ins", "kbd", "label", "legend", "li", "mark", "meter", "ol", "option", "p", "q", "progress", "rt", "ruby", "samp", "section", "select", "small", "strong", "tt", "u"}
	var result string
	for i := 0; i < 100; i++ {
		if domainParts := strings.Split(domain, "."); ((IsDoubleSuffixDomain(domain) && len(domainParts) == 3) || len(domainParts) == 2) && rand.Intn(100) < 20 {
			result = result + fmt.Sprintf(`<a href="%s" target="_blank">%s</a>`, schema+"://"+RandStr(3, 5)+"."+domain, RandStr(6, 16))
			continue
		}
		t := htmlTags[rand.Intn(len(htmlTags))]
		result = result + fmt.Sprintf(`<%s id="%s">%s</%s>`, t, RandStr(4, 8), RandStr(6, 16), t)
	}
	return "<div style=\"display:none\">" + result + "</div>"
}
func RandStr(minLength int, maxLength int) string {
	chars := []rune("ABCDEFGHIJKLNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	length := rand.Intn(maxLength-minLength) + minLength
	result := ""
	for i := 0; i < length; i++ {
		result = result + string(chars[rand.Intn(len(chars))])
	}
	return result

}
func readLinks() map[string][]string {
	result := make(map[string][]string)
	linkData, err := os.ReadFile("config/links.txt")
	if err != nil && len(linkData) <= 0 {
		return result
	}
	linkLines := strings.Split(strings.ReplaceAll(string(linkData), "\r", ""), "\n")
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
	data, err := os.ReadFile("config/config.json")
	if err != nil {
		return appConfig, err
	}

	err = json.Unmarshal(data, &appConfig)
	if err != nil {
		return appConfig, err
	}
	//关键字文件
	keywordData, err := os.ReadFile("config/keywords.txt")
	if err == nil && len(keywordData) > 0 {
		appConfig.Keywords = strings.Split(strings.Replace(string(keywordData), "\r", "", -1), "\n")
	}
	//统计js
	js, err := os.ReadFile("config/inject.js")
	if err == nil {
		appConfig.InjectJs = string(js)
	}
	//友情链接文本
	appConfig.FriendLinks = readLinks()
	appConfig.AdDomains = adDomains()

	return appConfig, nil
}
func adDomains() map[string]bool {
	adDomainData, err := os.ReadFile("config/ad_domains.txt")
	adDomains := make(map[string]bool)
	if err != nil || len(adDomainData) == 0 {
		return adDomains

	}
	domains := strings.Split(strings.ReplaceAll(string(adDomainData), "\r", ""), "\n")
	for _, domain := range domains {
		adDomains[domain] = true
	}
	return adDomains
}

func GetExpireDate() (string, error) {
	pubKey := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAsfUtexjm9RVM5CpijrNF
NDI4NfCyMIxW9q+/QaBXiNbqoguWYh1Mmkt+tal6QqObyvmufAbMfJpj0b+cGm96
KYgAOXUntYAKkTvQLQoQQl9aGY/rxEPuVu+nvN0zsVHrDteaWpMu+7O6OyYS0aKL
nWhCYpobTp6MTheMfnlMi7p2pJmGxyvUvZNvv6O6OZelOyr7Pb1FeYzpc/8+vkmK
BGnbyK6EVbZ5vwTaw/X2DI4uDOneKU2qVUyq2nd7pSvbX9aSuQZq1xwWhIXcEY6l
XzFBxZbhjXaZkaO2CWTHLwcKtSCCd3PkXNCRWQeHM4OelRZJajKSxwcWWTqbusGC
2wIDAQAB
-----END PUBLIC KEY-----`

	certBytes, err := os.ReadFile("config/auth.cert")
	if err != nil {
		return "", err
	}
	data, err := gorsa.PublicDecrypt(string(certBytes), pubKey)
	if err != nil {
		return "", errors.New("认证失败，无法解密," + err.Error())
	}
	return data, nil
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
	passBytes, err := os.ReadFile("config/passwd")
	if err != nil || len(passBytes) == 0 {
		userName, password := genUserAndPass()
		err = os.WriteFile("config/passwd", []byte(userName+":"+password), os.ModePerm)
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

func HtmlEntities(input string) string {
	var buffer bytes.Buffer
	for _, r := range input {
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

func IsDoubleSuffixDomain(host string) bool {
	suffixes := []string{"com.cn", "net.cn", "org.cn"}
	for _, suffix := range suffixes {
		if strings.Contains(host, suffix) {
			return true
		}
	}
	return false
}
func Escape(content string) string {
	content = strings.ReplaceAll(content, "&", "&amp;")
	content = strings.ReplaceAll(content, "'", "&#39;")
	content = strings.ReplaceAll(content, "<", "&lt;")
	content = strings.ReplaceAll(content, "\"", "&#34;")
	content = strings.ReplaceAll(content, "\r", "&#13;")
	return content
}
func IsUTF8(content []byte) bool {
	for i := len(content) - 1; i >= 0 && i > len(content)-4; i-- {
		b := content[i]
		if b < 0x80 {
			break
		}
		if utf8.RuneStart(b) {
			content = content[:i]
			break
		}
	}
	hasHighBit := false
	for _, c := range content {
		if c >= 0x80 {
			hasHighBit = true
			break
		}
	}
	if hasHighBit && utf8.Valid(content) {
		return true
	}
	return false
}
