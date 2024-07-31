package alist_v3

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

func (d *AListV3) login() error {
	var resp common.Resp[LoginResp]
	_, err := d.request("/auth/login", http.MethodPost, func(req *resty.Request) {
		req.SetResult(&resp).SetBody(base.Json{
			"username": d.Username,
			"password": d.Password,
		})
	})
	if err != nil {
		return err
	}
	d.Token = resp.Data.Token
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *AListV3) request(api, method string, callback base.ReqCallback, retry ...bool) ([]byte, error) {
	url := d.Address + "/api" + api
	log.Debugf("==========alist_v3/util.go request() url: %v", url)
	req := base.RestyClient.R()
	log.Debugf("==========alist_v3/util.go request() req before callback: %v", req)
	req.SetHeader("Authorization", d.Token)
	if callback != nil {
		callback(req)
	}
	log.Debugf("==========alist_v3/util.go request() req after callback: %v", req)
	res, err := req.Execute(method, url)
	log.Debugf("==========alist_v3/util.go request() res : %v", res)
	if err != nil {
		return nil, err
	}
	log.Debugf("[alist_v3] response body: %s", res.String())
	if res.StatusCode() >= 400 {
		return nil, fmt.Errorf("request failed, status: %s", res.Status())
	}
	code := utils.Json.Get(res.Body(), "code").ToInt()
	if code != 200 {
		if (code == 401 || code == 403) && !utils.IsBool(retry...) {
			err = d.login()
			if err != nil {
				return nil, err
			}
			return d.request(api, method, callback, true)
		}
		return nil, fmt.Errorf("request failed,code: %d, message: %s", code, utils.Json.Get(res.Body(), "message").ToString())
	}

	var responseBody map[string]interface{}
	err = json.Unmarshal(res.Body(), &responseBody)
	if err != nil {
		return nil, err
	}

	// 检查是否存在raw_url字段
	data, ok := responseBody["data"].(map[string]interface{})
	if ok {
		rawURL, ok := data["raw_url"].(string)
		if ok {
			// 解析原始URL和响应中的URL
			originalURL, err := url.Parse(url)
			if err != nil {
				return nil, err
			}
			responseURL, err := url.Parse(rawURL)
			if err != nil {
				return nil, err
			}

			// 如果响应中的URL缺少端口号，就把原始URL的端口号加上
			if responseURL.Port() == "" && originalURL.Port() != "" {
				responseURL.Host = responseURL.Host + ":" + originalURL.Port()
				data["raw_url"] = responseURL.String()
			}
		}
	}

	// 将修改后的响应体转换回字节切片
	modifiedBody, err := json.Marshal(responseBody)
	if err != nil {
		return nil, err
	}
	log.Debugf("==========alist_v3/util.go request() res : %v", modifiedBody)
	return modifiedBody, nil
}
