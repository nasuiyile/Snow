package tool

import (
	"fmt"
	"net/http"
	"net/url"
	. "snow/common"
	"strconv"
)

var RemoteHttp = "127.0.0.1:8111"

var Num = 100

func SendHttp(from string, target string, data []byte, k int) {
	if data[1] == UserMsg {
		values := url.Values{}
		values.Add("From", from)
		values.Add("Target", target)
		values.Add("Size", fmt.Sprintf("%d", len(data)))
		if data[0] == ColoringMsg || data[0] == RegularMsg || data[0] == ReliableMsg {
			values.Add("Id", string(data[TagLen+IpLen*2:TagLen+IpLen*2+TimeLen]))
		} else if data[0] == EagerPush {
			values.Add("Id", string(data[TagLen+IpLen:TagLen+IpLen+TimeLen]))
		} else {
			//其他的消息都没有附带ip
			values.Add("Id", string(data[TagLen:TagLen+TimeLen]))
		}
		values.Add("FanOut", strconv.Itoa(k))
		values.Add("Num", strconv.Itoa(Num))

		values.Add("MsgType", strconv.Itoa(int(data[0])))
		values.Add("Size", fmt.Sprintf("%d", len(data)))

		baseURL := "http://" + RemoteHttp + "/putRing"
		fullURL := fmt.Sprintf("%s?%s", baseURL, values.Encode())
		// 发送HTTP GET请求
		http.Get(fullURL)
	}

}
