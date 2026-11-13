package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/skip2/go-qrcode"
)

const (
	qrGenerateURL = "https://passport.bilibili.com/x/passport-login/web/qrcode/generate"
	qrPollURL     = "https://passport.bilibili.com/x/passport-login/web/qrcode/poll"
	cookieFile    = "cookies.txt"
	qrImageFile   = "qr.png"
)

type qrGenerateRes struct {
	Code int `json:"code"`
	Data struct {
		URL       string `json:"url"`
		QrcodeKey string `json:"qrcode_key"`
	} `json:"data"`
}

type qrPollRes struct {
	Code int `json:"code"`
	Data struct {
		URL  string `json:"url"`
		Code int    `json:"code"`
		Msg  string `json:"message"`
	} `json:"data"`
	Message string `json:"message"`
}

// loginClient 登录接口专用 http client(带 UA, 无 cookie)
var loginClient = &http.Client{Timeout: 30 * time.Second}

// loadCookies 从本地 cookies.txt 载入凭证
func loadCookies() (csrf string, sessdata string, err error) {
	data, err := os.ReadFile(cookieFile)
	if err != nil {
		return "", "", err
	}
	// 容错: 剥离 UTF-8 BOM(手编文件常见)
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var c struct {
		Csrf     string `json:"csrf"`
		Sessdata string `json:"sessdata"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return "", "", err
	}
	if c.Csrf == "" || c.Sessdata == "" {
		return "", "", errors.New("cookie file incomplete")
	}
	return c.Csrf, c.Sessdata, nil
}

// saveCookies 保存凭证到本地(权限 600)
func saveCookies(csrf string, sessdata string) {
	data, err := json.MarshalIndent(struct {
		Csrf     string `json:"csrf"`
		Sessdata string `json:"sessdata"`
	}{csrf, sessdata}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(cookieFile, data, 0600)
}

// qrLogin 扫码登录: 显示二维码 -> 轮询 -> 从 Set-Cookie 提取凭证
func qrLogin() (string, string, error) {
	logger.Info("===== 扫码登录 =====")
	logger.Info("正在获取二维码...")

	// 1. 请求二维码
	req, err := http.NewRequest("GET", qrGenerateURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := loginClient.Do(req)
	if err != nil {
		return "", "", err
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return "", "", err
	}
	var gen qrGenerateRes
	if err := json.Unmarshal(body, &gen); err != nil || gen.Code != 0 {
		return "", "", fmt.Errorf("failed to generate qrcode: %s", string(body))
	}

	// 2. 生成二维码: PNG 文件 + 终端 ASCII
	qr, err := qrcode.New(gen.Data.URL, qrcode.Medium)
	if err != nil {
		return "", "", err
	}
	_ = qr.WriteFile(256, qrImageFile)
	fmt.Println()
	fmt.Println(qr.ToSmallString(false))
	fmt.Println("请用 B 站手机 App 扫描上方二维码登录(或已弹出的 qr.png)")
	// 尝试用系统默认看图工具打开二维码图片
	_ = exec.Command("cmd", "/c", "start", "", qrImageFile).Start()

	// 3. 轮询扫码状态
	deadline := time.Now().Add(3 * time.Minute)
	scanned := false
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		pollURL := qrPollURL + "?qrcode_key=" + gen.Data.QrcodeKey
		req, err := http.NewRequest("GET", pollURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Referer", "https://www.bilibili.com/")
		res, err := loginClient.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			continue
		}
		var poll qrPollRes
		if err := json.Unmarshal(body, &poll); err != nil {
			continue
		}
		// 注意: 外层 code 恒为 0, 扫码状态在 data.code 中
		switch poll.Data.Code {
		case 0:
			// 登录成功, 从 Set-Cookie 提取凭证
			var csrf, sessdata string
			for _, c := range res.Cookies() {
				switch c.Name {
				case "bili_jct":
					csrf = c.Value
				case "SESSDATA":
					sessdata = c.Value
				}
			}
			_ = os.Remove(qrImageFile) // 二维码含登录 token, 用完即删
			if csrf == "" || sessdata == "" {
				return "", "", errors.New("login succeeded but cookies missing")
			}
			return csrf, sessdata, nil
		case 86090:
			if !scanned {
				scanned = true
				logger.Info("已扫码, 请在手机上确认登录...")
			}
		case 86038:
			_ = os.Remove(qrImageFile)
			return "", "", errors.New("二维码已过期, 请重新运行程序")
		case 86101:
			// 未扫码, 继续等待
		default:
			logger.Debug("poll status: ", poll.Data.Code, " ", poll.Data.Msg)
		}
	}
	_ = os.Remove(qrImageFile)
	return "", "", errors.New("扫码超时(3 分钟未确认), 请重新运行程序")
}
