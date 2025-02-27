package util

import (
	"encoding/json"
	"github.com/jweny/pocassist/pkg/cel/proto"
	"github.com/jweny/pocassist/pkg/conf"
	"github.com/valyala/fasthttp"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type clientDoer interface {
	DoTimeout(req *fasthttp.Request, resp *fasthttp.Response, t time.Duration) error
	DoRedirects(req *fasthttp.Request, resp *fasthttp.Response, maxRedirectsCount int) error
}

var (
	fasthttpClient   clientDoer
)

var (
	requestPool sync.Pool = sync.Pool{
		New: func() interface{} {
			return new(proto.Request)
		},
	}

	responsePool sync.Pool = sync.Pool{
		New: func() interface{} {
			return new(proto.Response)
		},
	}

	formatPool sync.Pool = sync.Pool{
		New: func() interface{} {
			return new(FormatString)
		},
	}
)

type ReqFormat struct {
	Req *fasthttp.Request
}

type RespFormat struct {
	Resp *fasthttp.Response
}

func (r *ReqFormat) FormatContent() string {
	reqRaw := formatPool.Get().(*FormatString)
	defer formatPut(reqRaw)
	reqRaw.Header = r.Req.Header.String()
	if len(r.Req.Body()) > 0 {
		reqRaw.Body = string(r.Req.Body())
	}
	reqContent, err := json.Marshal(reqRaw)
	if err != nil {
		return ""
	}
	return string(reqContent)
}

func (r *RespFormat) FormatContent() string {
	respRaw := formatPool.Get().(*FormatString)
	defer formatPut(respRaw)
	respRaw.Header = r.Resp.Header.String()
	if len(r.Resp.Body()) > 0 {
		respRaw.Body = string(r.Resp.Body())
	}
	respContent, err := json.Marshal(respRaw)
	if err != nil {
		return ""
	}
	return string(respContent)
}

func formatPut(f *FormatString) {
	if f == nil {
		return
	}
	f.Header = ""
	f.Body = ""
	formatPool.Put(f)
}

func RequestGet() *proto.Request {
	return requestPool.Get().(*proto.Request)
}

func RequestPut(r *proto.Request) {
	if r == nil {
		return
	}
	r.Reset()
	requestPool.Put(r)
}

func RespGet() *proto.Response {
	return responsePool.Get().(*proto.Response)
}

func ResponsePut(resp *proto.Response) {
	if resp == nil {
		return
	}
	resp.Reset()
	responsePool.Put(resp)
}

func ResponsesPut(responses []*proto.Response) {
	for _, item := range responses {
		ResponsePut(item)
	}
}

func ParseUrl(u *url.URL) *proto.UrlType {
	nu := &proto.UrlType{}
	nu.Scheme = u.Scheme
	nu.Domain = u.Hostname()
	nu.Host = u.Host
	nu.Port = u.Port()
	nu.Path = u.EscapedPath()
	nu.Query = u.RawQuery
	nu.Fragment = u.Fragment
	return nu
}

func ParseFasthttpResponse(originalResp *fasthttp.Response, req *fasthttp.Request) (*proto.Response, error) {
	resp := RespGet()
	header := make(map[string]string)
	resp.Status = int32(originalResp.StatusCode())
	u, err := url.Parse(req.URI().String())
	if err != nil {
		return nil, err
	}
	resp.Url = ParseUrl(u)

	headerContent := originalResp.Header.String()
	headers := strings.Split(headerContent, "\r\n")
	for _, v := range headers {
		values := strings.Split(v, ":")
		if len(values) != 2 {
			continue
		}
		k := values[0]
		v := values[1]
		header[k] = v
	}
	resp.Headers = header
	resp.ContentType = string(originalResp.Header.Peek("Content-Type"))

	resp.Body = make([]byte, len(originalResp.Body()))
	copy(resp.Body, originalResp.Body())
	return resp, nil
}

func DoFasthttpRequest(req *fasthttp.Request, redirect bool) (*proto.Response, error) {
	defer fasthttp.ReleaseRequest(req)
	bodyLen := len(req.Body())
	if bodyLen > 0 {
		req.Header.Set("Content-Length", strconv.Itoa(bodyLen))
		if string(req.Header.Peek("Content-Type")) == "" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	var err error

	if redirect {
		timeout := conf.GlobalConfig.HttpConfig.HttpTimeout
		err = fasthttpClient.DoTimeout(req, resp, time.Duration(timeout)*time.Second)
	} else {
		// 不接受跳转
		err = fasthttpClient.DoRedirects(req, resp, 0)
	}
	if err != nil {
		return nil, err
	}

	curResp, err := ParseFasthttpResponse(resp, req)
	// 添加请求和响应报文
	if err != nil {
		return nil, err
	}

	f := RespFormat{
		Resp: resp,
	}
	curResp.RespRaw = f.FormatContent()

	reqf := ReqFormat{
		Req: req,
	}
	curResp.ReqRaw = reqf.FormatContent()
	return curResp, err
}

func UrlTypeToString(u *proto.UrlType) string {
	var buf strings.Builder
	if u.Scheme != "" {
		buf.WriteString(u.Scheme)
		buf.WriteByte(':')
	}
	if u.Scheme != "" || u.Host != "" {
		if u.Host != "" || u.Path != "" {
			buf.WriteString("//")
		}
		if h := u.Host; h != "" {
			buf.WriteString(u.Host)
		}
	}
	path := u.Path
	if path != "" && path[0] != '/' && u.Host != "" {
		buf.WriteByte('/')
	}
	if buf.Len() == 0 {
		if i := strings.IndexByte(path, ':'); i > -1 && strings.IndexByte(path[:i], '/') == -1 {
			buf.WriteString("./")
		}
	}
	buf.WriteString(path)

	if u.Query != "" {
		buf.WriteByte('?')
		buf.WriteString(u.Query)
	}
	if u.Fragment != "" {
		buf.WriteByte('#')
		buf.WriteString(u.Fragment)
	}
	return buf.String()
}

func CopyRequest(req *http.Request, dstRequest *fasthttp.Request, data []byte) error {

	curURL := req.URL.String()
	dstRequest.SetRequestURI(curURL)
	dstRequest.Header.SetMethod(req.Method)

	for name, values := range req.Header {
		// Loop over all values for the name.
		for index, value := range values {
			if index > 0 {
				dstRequest.Header.Add(name, value)
			} else {
				dstRequest.Header.Set(name, value)
			}
		}
	}
	dstRequest.SetBodyRaw(data)
	return nil
}

func GenOriginalReq(url string) (*http.Request, error) {

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
	} else {
		url = "http://" + url
	}
	originalReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	originalReq.Header.Set("User-Agent", conf.GlobalConfig.HttpConfig.Headers.UserAgent)

	return originalReq, nil
}


