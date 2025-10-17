package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/FH-TianHe/BiliMux/api"
	"github.com/FH-TianHe/BiliMux/config"
)

// 扫码登录相关结构体
type QRCodeResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		URL      string `json:"url"`
		OAuthKey string `json:"oauth_key"`
	} `json:"data"`
}

type LoginStatusResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Status  int    `json:"status"` // 0: 未扫描, 1: 已扫描, 2: 已确认
		Message string `json:"message"`
		URL     string `json:"url"`
	} `json:"data"`
}

type LoginSuccessResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenInfo    struct {
			Mid   int    `json:"mid"`
			Uname string `json:"uname"`
		} `json:"token_info"`
		CookieInfo struct {
			Cookies []struct {
				Name     string `json:"name"`
				Value    string `json:"value"`
				HTTPOnly int    `json:"http_only"`
				Expires  int    `json:"expires"`
				Secure   int    `json:"secure"`
			} `json:"cookies"`
		} `json:"cookie_info"`
	} `json:"data"`
}

// 扫码登录状态
type LoginState struct {
	mu          sync.Mutex
	states      map[string]*LoginSession // oauth_key -> session
	activeUsers map[int]time.Time        // mid -> 最后活动时间
}

type LoginSession struct {
	OAuthKey    string
	QRCodeURL   string
	Status      int // 0: 未扫描, 1: 已扫描, 2: 已确认
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ScanTime    time.Time
	ConfirmTime time.Time
	Cookies     string
	Buvid3      string
}

var loginState = &LoginState{
	states:      make(map[string]*LoginSession),
	activeUsers: make(map[int]time.Time),
}

// 添加登录会话
func AddLoginSession(oauthKey string, session *LoginSession) {
	loginState.mu.Lock()
	defer loginState.mu.Unlock()
	loginState.states[oauthKey] = session
}

// 获取登录会话
func GetLoginSession(oauthKey string) (*LoginSession, bool) {
	loginState.mu.Lock()
	defer loginState.mu.Unlock()
	session, exists := loginState.states[oauthKey]
	return session, exists
}

// 生成随机的oauth_key
func GenerateOAuthKey() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// 清理过期会话
func CleanExpiredSessions() {
	for {
		time.Sleep(5 * time.Minute)
		loginState.mu.Lock()
		for key, session := range loginState.states {
			if time.Now().After(session.ExpiresAt) {
				delete(loginState.states, key)
			}
		}
		loginState.mu.Unlock()
	}
}

// 获取登录后的Cookie
func FetchLoginCookies(session *LoginSession) error {
	// 调用B站API获取登录信息
	apiURL := "https://passport.bilibili.com/x/passport-login/oauth2/info"
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", "oauthKey="+session.OAuthKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var loginResp LoginSuccessResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return err
	}

	if loginResp.Code != 0 {
		return fmt.Errorf("获取登录信息失败: %s", loginResp.Message)
	}

	// 构建Cookie字符串
	var cookies []string
	for _, cookie := range loginResp.Data.CookieInfo.Cookies {
		cookies = append(cookies, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
	}
	session.Cookies = strings.Join(cookies, "; ")
	config.SetCookie(session.Cookies)

	// 获取Buvid3
	buvid3, err := api.GetRealBuvid3()
	if err != nil {
		return fmt.Errorf("获取Buvid3失败: %v", err)
	}
	session.Buvid3 = buvid3
	config.SetBuvid3(buvid3)

	// 记录活跃用户
	if loginResp.Data.TokenInfo.Mid != 0 {
		loginState.mu.Lock()
		loginState.activeUsers[loginResp.Data.TokenInfo.Mid] = time.Now()
		loginState.mu.Unlock()
	}

	return nil
}
