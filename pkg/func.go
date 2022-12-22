package pkg

import (
	"context"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

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
			if ipList != nil && len(ipList) > 0 {
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
