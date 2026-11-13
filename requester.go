package main

import (
	"encoding/json"
	"errors"
	"github.com/mitchellh/mapstructure"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// APIError bilibili 接口返回非 0 code 时的错误, 保留 code 便于定位问题
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return "bilibili api error " + strconv.Itoa(e.Code) + ": " + e.Message
}

func getJuryInfo() (JuryInfo, error) {
	resData, err := request("GET", JuryURL, make(map[string]string))
	if err != nil {
		return JuryInfo{}, err
	}
	var ji JuryInfo
	err = mapstructure.Decode(resData.Data, &ji)
	if err != nil {
		return JuryInfo{}, err
	}
	return ji, nil
}

func getNext() (string, error) {
	resData, err := request("GET", NextURL, make(map[string]string))
	if err != nil {
		return "", err
	}
	obj := resData.Data["case_id"]
	switch obj.(type) {
	case string:
		return obj.(string), nil
	default:
		return "", errors.New("case id must be string")
	}
}

func getCaseInfo(caseId string) (caseInfo, error) {
	resData, err := request("GET", InfoURL, map[string]string{
		"case_id": caseId,
	})
	if err != nil {
		return caseInfo{}, err
	}
	var ci caseInfo
	err = mapstructure.Decode(resData.Data, &ci)
	if err != nil {
		return caseInfo{}, err
	}
	return ci, nil
}

func getFullOpinion(caseId string) (opinion, error) {
	o, err := getOpinion(caseId, 1, 20)
	if err != nil {
		return opinion{}, err
	}
	pg := 2
	for len(o.List) < o.Total {
		nextPageOpinion, err := getOpinion(caseId, pg, 20)
		if err != nil {
			return opinion{}, err
		}
		// 保护: total 可能包含已删除的意见, 若返回空页则跳出, 避免死循环
		if len(nextPageOpinion.List) == 0 {
			break
		}
		o.List = append(o.List, nextPageOpinion.List...)
		pg++
	}
	return o, nil
}

func getOpinion(caseId string, pg int, ps int) (opinion, error) {
	resData, err := request("GET", opinionURL, map[string]string{
		"case_id": caseId,
		"pg":      strconv.Itoa(pg),
		"ps":      strconv.Itoa(ps),
	})
	if err != nil {
		return opinion{}, err
	}
	var o opinion
	err = mapstructure.Decode(resData.Data, &o)
	if err != nil {
		return opinion{}, err
	}
	return o, nil
}

func postVote(caseId string, vote int) (ResData, error) {
	insiders := strconv.Itoa(rand.Intn(2))
	return request("POST", voteURL, map[string]string{
		"case_id":   caseId,
		"vote":      strconv.Itoa(vote),
		"insiders":  insiders,
		"content":   "",
		"anonymous": "1",
		"csrf":      csrf,
	})
}

func request(method string, api string, param map[string]string) (ResData, error) {
	return doRequest(method, api, param, true)
}

// doRequest 执行实际请求; allowRelogin 控制 -101 时是否自动重新扫码登录(防止无限递归)
func doRequest(method string, api string, param map[string]string, allowRelogin bool) (ResData, error) {
	var req *http.Request
	var err error
	if method == "POST" {
		// POST 参数放入 form body(与网页端一致), csrf 不应暴露在 URL 中
		form := url.Values{}
		for k, v := range param {
			form.Set(k, v)
		}
		req, err = http.NewRequest(method, api, strings.NewReader(form.Encode()))
		if err != nil {
			return ResData{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest(method, api, nil)
		if err != nil {
			return ResData{}, err
		}
		query := req.URL.Query()
		for k, v := range param {
			query.Add(k, v)
		}
		req.URL.RawQuery = query.Encode()
	}
	addHeader(&req.Header)
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return ResData{}, err
	}
	defer res.Body.Close()
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return ResData{}, err
	}
	logger.Debug(string(bytes))
	var rd ResData
	err = json.Unmarshal(bytes, &rd)
	if err != nil {
		return ResData{}, err
	}
	if rd.Code != 0 {
		// 凭证失效: 自动扫码重登并重试一次(仅非命令行参数模式)
		if rd.Code == -101 && allowRelogin && relogin() {
			return doRequest(method, api, param, false)
		}
		return ResData{}, &APIError{Code: rd.Code, Message: rd.Message}
	}
	return rd, nil
}

// relogin 重新扫码登录, 成功后更新全局凭证并保存; 命令行参数模式不自动重登
func relogin() bool {
	if len(os.Args) >= 3 {
		return false
	}
	logger.Warn("凭证已失效, 自动重新扫码登录...")
	c, s, err := qrLogin()
	if err != nil {
		logger.Warn("重新登录失败: ", err.Error())
		return false
	}
	csrf, sessdata = c, s
	saveCookies(csrf, sessdata)
	logger.Info("重新登录成功, 凭证已更新到 ", cookieFile)
	return true
}

// userAgent 统一的浏览器 UA
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36"

func addHeader(header *http.Header) {
	header.Set("cookie", "bili_jct="+csrf+"; SESSDATA="+sessdata+";")
	header.Set("User-Agent", userAgent)
	header.Set("Referer", "https://www.bilibili.com/")
	header.Set("Origin", "https://www.bilibili.com")
}
