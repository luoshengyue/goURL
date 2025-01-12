package utils

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/fatih/color"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strings"
	"time"
)

type headers []string

func (h headers) String() string {
	var o []string
	for _, v := range h {
		o = append(o, "-H "+v)
	}
	return strings.Join(o, " ")
}

func (h *headers) Set(v string) error {
	*h = append(*h, v)
	return nil
}

func (h headers) Len() int      { return len(h) }
func (h headers) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h headers) Less(i, j int) bool {
	a, b := h[i], h[j]

	// server always sorts at the top
	if a == "Server" {
		return true
	}
	if b == "Server" {
		return false
	}

	endtoend := func(n string) bool {
		// https://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html#sec13.5.1
		switch n {
		case "Connection",
			"Keep-Alive",
			"Proxy-Authenticate",
			"Proxy-Authorization",
			"TE",
			"Trailers",
			"Transfer-Encoding",
			"Upgrade":
			return false
		default:
			return true
		}
	}

	x, y := endtoend(a), endtoend(b)
	if x == y {
		// both are of the same class
		return a < b
	}
	return x
}

var (
	// Command line flags
	HttpMethod       string // http method
	HttpResponseHead bool   // response head
	HttpConnectInfo  bool   // connect information

	ShowVersion bool	// show program version

	Version = "Dev"
)

func printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(color.Output, format, a...)
}

func grayscale(code color.Attribute) func(string, ...interface{}) string {
	return color.New(code + 232).SprintfFunc()
}

func VisitURL(url *url.URL) error {
	// TODO: data body have not set flag
	req, err := newRequest(HttpMethod, url, "")
	// We add req User-Agent
	// // TODO: modify this param later
	req.Header.Add("User-Agent", "curl/7.77.0")
	if err != nil {
		return err
	}

	// TODO: count time cost

	trace := &httptrace.ClientTrace{
		ConnectDone: func(net, addr string, err error) {
			if err != nil {
				log.Fatalf("unable to connect to host %v: %v", addr, err)
			}

			printf("\n%s%s\n", color.GreenString("Connected to "), color.CyanString(addr))
		},
	}

	req = req.WithContext(httptrace.WithClientTrace(context.Background(), trace))

	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		MaxIdleConns: 100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	// TODO: choose IPv4 or IPv6

	switch url.Scheme {
	case "https":
		host, _, err := net.SplitHostPort(req.Host)
		if err != nil {
			host = req.Host
		}

		tr.TLSClientConfig = &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		}
	}

	client := &http.Client{
		Transport: tr,
	}

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return errors.New(color.HiRedString("failed to read response:", err))
	}
	// Print SSL/TLS version which is used for connection
	connectedVia := "plaintext"
	if resp.TLS != nil {
		switch resp.TLS.Version {
		case tls.VersionTLS12:
			connectedVia = "TLSv1.2"
		case tls.VersionTLS13:
			connectedVia = "TLSv1.3"
		}
	}
	printf("\n%s %s\n", color.GreenString("Connected via"), color.CyanString("%s", connectedVia))

	// show connect-info
	if HttpConnectInfo {
		showRequestInfo(req)
		printf("%s\n", grayscale(14)("*Get response from server"))
		showResponseHeader(resp)
	}

	// show response head and source code
	if HttpResponseHead {
		if !HttpConnectInfo {
			showResponseHeader(resp)
		}
		// this func is show full response body.
		showResponseBody(resp)
	} else {
		showBriefResponse(resp)
	}
	return nil
}

func newRequest(method string, url *url.URL, body string) (*http.Request, error) {
	req, err := http.NewRequest(method, url.String(), createBody(body))
	if err != nil {
		return nil, errors.New(color.HiRedString("Unable to create request:", err))
	}
	// TODO: add headers for request
	return req, nil
}

func createBody(body string) io.Reader {
	return strings.NewReader(body)
}

func showRequestInfo(req *http.Request)  {
	printf(">%s %s\n", grayscale(14)(req.Method), grayscale(14)(req.Proto))
	printf(">%s:%s\n", grayscale(14)("Host"), color.CyanString(req.Host))
	userAgent := req.UserAgent()
	if userAgent == "" {
		userAgent = "*"
	}
	printf(">%s:%s\n", grayscale(14)("User-Agent"), color.CyanString(userAgent))
	accept := req.Header.Get("Accept")
	if accept == "" {
		accept = "*/*"
	}
	printf(">%s:%s\n", grayscale(14)("Accept"), color.CyanString(accept))
}

func showResponseHeader(resp *http.Response)  {
	names := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		names = append(names, k)
	}
	sort.Sort(headers(names))
	for _, k := range names {
		printf("<%s %s\n", grayscale(14)(k+":"), color.CyanString(strings.Join(resp.Header[k], ",")))
	}
}

// show brief response body.
func showBriefResponse(resp *http.Response)  {
	s, _ := ioutil.ReadAll(resp.Body)
	body := strings.Split(string(s), "\n")
	// we only show first and last five lines.
	show := append(body[:5], body[len(body) - 3:]...)
	printf("%s", grayscale(14)("Body:"))
	for _, s := range show {
		printf("%s\n", color.CyanString(s))
	}
}

// Show full response.
func showResponseBody(resp *http.Response)  {
	s, _ := ioutil.ReadAll(resp.Body)
	printf("%s %s\n", grayscale(14)("Body:"), color.CyanString(string(s)))
}