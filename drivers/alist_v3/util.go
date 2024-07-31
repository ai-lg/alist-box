package alist_v3

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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
	requestURL := d.Address + "/api" + api
	log.Debugf("==========alist_v3/util.go request() url: %v", requestURL)
	req := base.RestyClient.R()
	req.SetHeader("Authorization", d.Token)
	if callback != nil {
		callback(req)
	}
	res, err := req.Execute(method, requestURL)
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
			originalURL, err := url.Parse(requestURL)
			if err != nil {
				return nil, err
			}
			log.Debugf("==========alist_v3/util.go request() originalURL : %v", originalURL)
			responseURL, err := url.Parse(rawURL)
			if err != nil {
				return nil, err
			}
			log.Debugf("==========alist_v3/util.go request() responseURL : %v", responseURL)

			log.Debugf("==========alist_v3/util.go request() responseURL.Port : %v", responseURL.Port())
			log.Debugf("==========alist_v3/util.go request() originalURL.Port : %v", originalURL.Port())
			if responseURL.Port() == "" && originalURL.Port() != "" {
				responseURL.Host = responseURL.Host + ":" + originalURL.Port()
				data["raw_url"] = responseURL.String()
			}
		} else {
			// 如果data中没有raw_url，返回原始的响应体
			return res.Body(), nil
		}
	} else {
		// 如果data中没有raw_url，返回原始的响应体
		return res.Body(), nil
	}

	// 将修改后的响应体转换回字节切片
	modifiedBody, err := json.Marshal(responseBody)
	if err != nil {
		return nil, err
	}
	log.Debugf("==========alist_v3/util.go request() modifiedBody : %s", string(modifiedBody))
	return modifiedBody, nil
}
