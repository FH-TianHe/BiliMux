package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/skip2/go-qrcode"

	"github.com/FH-TianHe/BiliMux/api"
	"github.com/FH-TianHe/BiliMux/config"
	"github.com/FH-TianHe/BiliMux/manager"
	"github.com/FH-TianHe/BiliMux/protocol"
	"github.com/FH-TianHe/BiliMux/utils"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 代理处理函数
func ProxyHandler(cm *manager.ConnectionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 获取房间ID
		roomIDStr := r.URL.Query().Get("room_id")
		if roomIDStr == "" {
			http.Error(w, "缺少room_id参数", http.StatusBadRequest)
			return
		}

		var roomID int
		if _, err := fmt.Sscanf(roomIDStr, "%d", &roomID); err != nil {
			http.Error(w, "无效的房间ID", http.StatusBadRequest)
			return
		}

		// 升级客户端连接到WebSocket
		clientConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("升级客户端连接失败:", err)
			cm.IncrementErrors()
			return
		}
		defer clientConn.Close()

		// 获取真实房间ID
		realRoomID, err := api.GetRealRoomID(roomID)
		if err != nil {
			log.Printf("获取真实房间ID失败: %v", err)
			cm.IncrementErrors()
			return
		}

		// 获取弹幕信息
		token, hosts, err := api.GetDanmuInfo(realRoomID)
		if err != nil {
			log.Printf("获取弹幕信息失败: %v", err)
			cm.IncrementErrors()
			return
		}

		// 随机选择一个服务器
		host := hosts[rand.Intn(len(hosts))]
		hostURL := fmt.Sprintf("wss://%s:%d/sub", host["host"], host["wss_port"])

		// 连接到B站服务器
		biliConn, _, err := websocket.DefaultDialer.Dial(hostURL, nil)
		if err != nil {
			log.Println("连接B站服务器失败:", err)
			cm.IncrementErrors()
			return
		}
		defer biliConn.Close()

		// 创建认证包
		buvid3 := config.GetConfig().Buvid3
		authPacket, err := protocol.CreateAuthPacket(realRoomID, token, buvid3)
		if err != nil {
			log.Println("创建认证包失败:", err)
			cm.IncrementErrors()
			return
		}

		// 发送认证包
		if err := biliConn.WriteMessage(websocket.BinaryMessage, authPacket); err != nil {
			log.Println("发送认证包失败:", err)
			cm.IncrementErrors()
			return
		}

		// 启动心跳
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go protocol.HandleHeartbeat(biliConn, cm)

		// 添加到连接管理器
		if !cm.Add(clientConn, cancel) {
			log.Println("达到最大连接数限制")
			return
		}
		defer cm.Remove(clientConn)

		log.Printf("已建立代理: %s <-> %s", r.RemoteAddr, hostURL)

		// 双向转发数据
		var wg sync.WaitGroup
		wg.Add(2)

		// 客户端 -> B站服务器
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					msgType, msg, err := clientConn.ReadMessage()
					if err != nil {
						return
					}
					if err := biliConn.WriteMessage(msgType, msg); err != nil {
						return
					}
					cm.IncrementMessages()
				}
			}
		}()

		// B站服务器 -> 客户端
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					msgType, msg, err := biliConn.ReadMessage()
					if err != nil {
						return
					}
					if err := clientConn.WriteMessage(msgType, msg); err != nil {
						return
					}
					cm.IncrementMessages()
				}
			}
		}()

		wg.Wait()
		log.Printf("连接关闭: %s", r.RemoteAddr)
	}
}

// 统计处理函数
func StatsHandler(cm *manager.ConnectionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := cm.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// 二维码处理函数
func QRCodeHandler(w http.ResponseWriter, r *http.Request) {
	// 生成随机的oauth_key
	oauthKey := utils.GenerateOAuthKey()

	// 调用B站API获取二维码URL
	apiURL := "https://passport.bilibili.com/x/passport-tv-login/qrcode/auth_code"
	resp, err := http.PostForm(apiURL, url.Values{
		"local_id": {"0"},
		"ts":       {fmt.Sprintf("%d", time.Now().Unix())},
	})
	if err != nil {
		http.Error(w, "申请二维码失败", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var qrResp utils.QRCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&qrResp); err != nil {
		http.Error(w, "解析响应失败", http.StatusInternalServerError)
		return
	}

	if qrResp.Code != 0 {
		http.Error(w, qrResp.Message, http.StatusInternalServerError)
		return
	}

	// 生成二维码图片
	qr, err := qrcode.New(qrResp.Data.URL, qrcode.Medium)
	if err != nil {
		http.Error(w, "生成二维码失败", http.StatusInternalServerError)
		return
	}

	// 将二维码图片转换为PNG
	img := qr.Image(256)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		http.Error(w, "生成二维码图片失败", http.StatusInternalServerError)
		return
	}

	// 创建登录会话
	session := &utils.LoginSession{
		OAuthKey:  oauthKey,
		QRCodeURL: qrResp.Data.URL,
		Status:    0,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	// 保存会话状态
	utils.AddLoginSession(oauthKey, session)

	// 返回二维码图片和oauth_key
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"oauth_key": oauthKey,
		"qrcode":    "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
	})
}

// 检查登录状态处理函数
func CheckLoginHandler(w http.ResponseWriter, r *http.Request) {
	oauthKey := r.URL.Query().Get("oauth_key")
	if oauthKey == "" {
		http.Error(w, "缺少oauth_key参数", http.StatusBadRequest)
		return
	}

	session, exists := utils.GetLoginSession(oauthKey)
	if !exists {
		http.Error(w, "无效的oauth_key", http.StatusBadRequest)
		return
	}

	// 检查是否过期
	if time.Now().After(session.ExpiresAt) {
		http.Error(w, "二维码已过期", http.StatusBadRequest)
		return
	}

	// 调用B站API检查登录状态
	apiURL := "https://passport.bilibili.com/x/passport-tv-login/qrcode/poll"
	resp, err := http.PostForm(apiURL, url.Values{
		"oauth_key": {oauthKey},
	})
	if err != nil {
		http.Error(w, "检查登录状态失败", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var statusResp utils.LoginStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		http.Error(w, "解析响应失败", http.StatusInternalServerError)
		return
	}

	// 更新会话状态
	session.Status = statusResp.Data.Status
	if statusResp.Data.Status == 1 && session.ScanTime.IsZero() {
		session.ScanTime = time.Now()
	} else if statusResp.Data.Status == 2 && session.ConfirmTime.IsZero() {
		session.ConfirmTime = time.Now()

		// 登录成功，获取Cookie
		if err := utils.FetchLoginCookies(session); err != nil {
			log.Printf("获取登录Cookie失败: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  statusResp.Data.Status,
		"message": statusResp.Data.Message,
	})
}
