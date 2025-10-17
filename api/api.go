package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FH-TianHe/BiliMux/config"
)

// 获取真实房间ID
func GetRealRoomID(roomID int) (int, error) {
	apiURL := "https://api.live.bilibili.com/room/v1/Room/get_info"
	params := url.Values{}
	params.Add("room_id", fmt.Sprintf("%d", roomID))

	resp, err := http.Get(apiURL + "?" + params.Encode())
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			RoomID int `json:"room_id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if result.Code != 0 {
		return 0, fmt.Errorf("获取真实房间ID失败: %d", result.Code)
	}

	return result.Data.RoomID, nil
}

// 获取弹幕信息
func GetDanmuInfo(realRoomID int) (string, []map[string]interface{}, error) {
	apiURL := "https://api.live.bilibili.com/xlive/web-room/v1/index/getDanmuInfo"
	params := url.Values{}
	params.Add("id", fmt.Sprintf("%d", realRoomID))

	req, err := http.NewRequest("GET", apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return "", nil, err
	}

	// 添加Cookie
	if config.GetConfig().Cookie != "" {
		req.Header.Set("Cookie", config.GetConfig().Cookie)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			Token    string                   `json:"token"`
			HostList []map[string]interface{} `json:"host_list"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", nil, err
	}

	if result.Code != 0 {
		return "", nil, fmt.Errorf("获取弹幕信息失败: %d", result.Code)
	}

	return result.Data.Token, result.Data.HostList, nil
}

// 获取Buvid3
func GetRealBuvid3() (string, error) {
	apis := []string{
		"https://api.bilibili.com/x/web-frontend/getbuvid",
		"https://api.bilibili.com/x/frontend/finger/spi",
	}

	for _, api := range apis {
		req, err := http.NewRequest("GET", api, nil)
		if err != nil {
			continue
		}

		// 添加Cookie
		if config.GetConfig().Cookie != "" {
			req.Header.Set("Cookie", config.GetConfig().Cookie)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		// 尝试解析第一个接口格式
		var buvidResp struct {
			Code int `json:"code"`
			Data struct {
				Buvid string `json:"buvid"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &buvidResp); err == nil && buvidResp.Code == 0 {
			return buvidResp.Data.Buvid, nil
		}

		// 尝试解析第二个接口格式
		var spiResp struct {
			Code int `json:"code"`
			Data struct {
				B3 string `json:"b_3"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &spiResp); err == nil && spiResp.Code == 0 {
			return spiResp.Data.B3, nil
		}
	}

	return "", fmt.Errorf("无法获取Buvid3")
}
