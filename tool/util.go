package tool

import (
	"fmt"
	"net/http"
	"net/url"
)

func SendHttp(from string, target string, data []byte) {
	values := url.Values{}
	values.Add("From", from)
	values.Add("Target", target)
	values.Add("Size", fmt.Sprintf("%d", len(data)))

	// 构建完整的URL，包括查询参数
	baseURL := "http://127.0.0.1:8111/putRing"
	fullURL := fmt.Sprintf("%s?%s", baseURL, values.Encode())

	// 发送HTTP GET请求
	http.Get(fullURL)
}
