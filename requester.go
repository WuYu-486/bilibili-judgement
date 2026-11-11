package main

import (
	"encoding/json"
	"errors"
	"github.com/mitchellh/mapstructure"
	"io"
	"math/rand"
	"net/http"
	"net/url"
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
		return ResData{}, &APIError{Code: rd.Code, Message: rd.Message}
	}
	return rd, nil
}

func addHeader(header *http.Header) {
	header.Set("cookie", "bili_jct="+csrf+"; SESSDATA="+sessdata+";")
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36")
	header.Set("Referer", "https://www.bilibili.com/")
	header.Set("Origin", "https://www.bilibili.com")
}
